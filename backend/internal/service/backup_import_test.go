//go:build unit

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testBackupZIP(t *testing.T, sql []byte, meta *localZIPManifest, extra string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entry, err := archive.Create("database.sql")
	require.NoError(t, err)
	_, err = entry.Write(sql)
	require.NoError(t, err)
	if meta != nil {
		entry, err = archive.Create("manifest.json")
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(entry).Encode(meta))
	}
	if extra != "" {
		_, err = archive.Create(extra)
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	return buffer.Bytes()
}

func TestBackupImportRoundTripWithoutExistingRecords(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	source, _, sourceDir := localBackupTestService(t)
	backup, err := source.startLocalBackup(t.Context(), "manual", 0, nil, false)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(sourceDir, backup.FileName))
	require.NoError(t, err)
	target, dumper, _ := localBackupTestService(t)
	imported, err := target.ImportLocalZIP(t.Context(), bytes.NewReader(data))
	require.NoError(t, err)
	require.True(t, imported.Portable)
	require.Equal(t, "import", imported.TriggeredBy)
	require.Empty(t, dumper.restored, "upload must not restore before password confirmation")
	_, err = target.StartRestore(t.Context(), imported.ID)
	require.NoError(t, err)
	target.wg.Wait()
	require.Equal(t, dumper.dumpData, dumper.restored)
	finished, err := target.GetBackupRecord(t.Context(), imported.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", finished.RestoreStatus)
}

func TestBackupImportRejectsCorruptionAndUnsafeEntries(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	svc, dumper, _ := localBackupTestService(t)
	sql := []byte("SELECT 42;\n")
	sum := sha256.Sum256(sql)
	meta := localZIPManifest{Format: "sub2api", Version: 1, SQLSHA256: hex.EncodeToString(sum[:]), KeysInDatabase: true}
	for _, extra := range []string{"../escape", "/absolute", "database.sql", "manifest.json", "unknown.txt"} {
		_, err := svc.ImportLocalZIP(t.Context(), bytes.NewReader(testBackupZIP(t, sql, &meta, extra)))
		require.Error(t, err)
	}
	meta.SQLSHA256 = strings.Repeat("0", 64)
	_, err := svc.ImportLocalZIP(t.Context(), bytes.NewReader(testBackupZIP(t, sql, &meta, "")))
	require.ErrorContains(t, err, "checksum mismatch")
	_, err = svc.ImportLocalZIP(t.Context(), strings.NewReader("not a ZIP"))
	require.Error(t, err)
	require.Empty(t, dumper.restored)
	files, err := os.ReadDir(filepath.Join(os.Getenv("DATA_DIR"), "backup-imports"))
	require.NoError(t, err)
	require.Empty(t, files, "failed imports must remove their uploaded files")
}

func TestBackupImportLegacyAndCancellation(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	svc, _, _ := localBackupTestService(t)
	data := testBackupZIP(t, []byte("SELECT 1;"), nil, "")
	record, err := svc.ImportLocalZIP(t.Context(), bytes.NewReader(data))
	require.NoError(t, err)
	require.False(t, record.Portable)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = svc.ImportLocalZIP(ctx, bytes.NewReader(data))
	require.ErrorIs(t, err, context.Canceled)
}

func TestBackupImportRejectsCRCBeforeRestore(t *testing.T) {
	// Stored data makes it possible to corrupt SQL without damaging the ZIP directory.
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	writer, err := archive.CreateHeader(&zip.FileHeader{Name: "database.sql", Method: zip.Store})
	require.NoError(t, err)
	_, err = io.WriteString(writer, "SELECT corruption_marker;")
	require.NoError(t, err)
	require.NoError(t, archive.Close())
	data := append([]byte(nil), buffer.Bytes()...)
	index := bytes.Index(data, []byte("corruption_marker"))
	require.NotEqual(t, -1, index)
	data[index] = 'x'
	_, _, err = validateLocalZIP(t.Context(), bytes.NewReader(data), int64(len(data)))
	require.Error(t, err)
}

func TestBackupImportTamperingFailsBeforeDatabaseRestore(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	svc, dumper, _ := localBackupTestService(t)
	record, err := svc.ImportLocalZIP(t.Context(), bytes.NewReader(testBackupZIP(t, []byte("SELECT 1;"), nil, "")))
	require.NoError(t, err)
	meta := localZIPManifest{Format: "sub2api", Version: 1, SQLSHA256: strings.Repeat("0", 64), KeysInDatabase: true}
	require.NoError(t, os.WriteFile(filepath.Join(record.LocalDirectory, record.FileName), testBackupZIP(t, []byte("SELECT changed;"), &meta, ""), 0600))
	_, err = svc.StartRestore(t.Context(), record.ID)
	require.NoError(t, err)
	svc.wg.Wait()
	record, err = svc.GetBackupRecord(t.Context(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", record.RestoreStatus)
	require.Empty(t, dumper.restored)
}
