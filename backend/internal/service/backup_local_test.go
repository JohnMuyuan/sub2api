//go:build unit

package service

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func localBackupTestService(t *testing.T) (*BackupService, *mockDumper, string) {
	t.Helper()
	dumper := &mockDumper{dumpData: []byte("CREATE TABLE example(id int);\nINSERT INTO example VALUES (42);\n")}
	svc := newTestBackupService(newMockSettingRepo(), dumper, newMockObjectStore())
	t.Cleanup(svc.Stop)
	directory := filepath.Join(t.TempDir(), "backups")
	_, err := svc.UpdateLocalConfig(t.Context(), BackupLocalConfig{Directory: directory})
	require.NoError(t, err)
	return svc, dumper, directory
}

func TestLocalZIPBackupRoundTripAndDirectoryChange(t *testing.T) {
	svc, dumper, directory := localBackupTestService(t)
	record, err := svc.StartBackup(t.Context(), "manual", 14, "local")
	require.NoError(t, err)
	require.Equal(t, "running", record.Status)
	svc.wg.Wait()
	record, err = svc.GetBackupRecord(t.Context(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", record.Status)
	require.Equal(t, "local", record.Storage)
	require.NotEmpty(t, record.ExpiresAt)
	archive, err := zip.OpenReader(filepath.Join(directory, record.FileName))
	require.NoError(t, err)
	require.Len(t, archive.File, 1)
	require.Equal(t, "database.sql", archive.File[0].Name)
	reader, err := archive.File[0].Open()
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, dumper.dumpData, data)
	require.NoError(t, reader.Close())
	require.NoError(t, archive.Close())
	// Existing records keep their original location after changing the setting.
	_, err = svc.UpdateLocalConfig(t.Context(), BackupLocalConfig{Directory: t.TempDir()})
	require.NoError(t, err)
	file, _, err := svc.OpenLocalBackup(t.Context(), record.ID)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.NoError(t, svc.RestoreBackup(t.Context(), record.ID))
	require.Equal(t, dumper.dumpData, dumper.restored)
	_, err = svc.StartRestore(t.Context(), record.ID)
	require.NoError(t, err)
	svc.wg.Wait()
	restored, err := svc.GetBackupRecord(t.Context(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", restored.RestoreStatus)
	require.NoError(t, svc.DeleteBackup(t.Context(), record.ID))
	_, err = os.Stat(filepath.Join(directory, record.FileName))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLocalZIPBackupScheduledRetention(t *testing.T) {
	svc, _, directory := localBackupTestService(t)
	ctx := t.Context()
	require.NoError(t, svc.settingRepo.Set(ctx, settingKeyBackupSchedule, `{"enabled":true,"storage":"local","retain_days":14,"retain_count":1}`))
	svc.runScheduledBackup()
	svc.runScheduledBackup()
	records, err := svc.ListBackups(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "scheduled", records[0].TriggeredBy)
	require.Equal(t, "completed", records[0].Status)
	files, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Len(t, files, 1)
}

func TestLocalZIPBackupFailureRemovesIncompleteFiles(t *testing.T) {
	svc, dumper, directory := localBackupTestService(t)
	dumper.dumpErr = errors.New("dump failed")
	record, err := svc.startLocalBackup(t.Context(), "manual", 0, nil, false)
	require.ErrorContains(t, err, "dump failed")
	require.Equal(t, "failed", record.Status)
	files, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, files)
	dumper.dumpErr = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = svc.startLocalBackup(ctx, "manual", 0, nil, false)
	require.Error(t, err)
	files, err = os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestLocalZIPBackupRejectsInvalidPaths(t *testing.T) {
	svc, _, _ := localBackupTestService(t)
	for _, path := range []string{"", "relative/path", "../escape"} {
		_, err := svc.UpdateLocalConfig(t.Context(), BackupLocalConfig{Directory: path})
		require.Error(t, err)
	}
	root, err := localBackupRoot(&BackupRecord{Storage: "local", LocalDirectory: t.TempDir(), ID: "id", FileName: "../sub2api_id.zip"})
	require.Error(t, err)
	require.Nil(t, root)
	_, err = svc.StartBackup(t.Context(), "manual", 0, "unsupported")
	require.Error(t, err)
}

type localBackupPanicDumper struct{ DBDumper }

func (localBackupPanicDumper) Dump(context.Context) (io.ReadCloser, error) {
	panic("test dump panic")
}

func TestLocalZIPBackupRecoversWorkerPanic(t *testing.T) {
	svc, _, directory := localBackupTestService(t)
	svc.dumper = localBackupPanicDumper{}
	record, err := svc.StartBackup(t.Context(), "manual", 0, "local")
	require.NoError(t, err)
	svc.wg.Wait()
	record, err = svc.GetBackupRecord(t.Context(), record.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", record.Status)
	require.Contains(t, record.ErrorMsg, "test dump panic")
	files, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestLocalZIPBackupBlocksConcurrentRestore(t *testing.T) {
	svc, _, _ := localBackupTestService(t)
	svc.backingUp = true
	_, err := svc.StartRestore(t.Context(), "any")
	require.ErrorIs(t, err, ErrBackupInProgress)
	require.ErrorIs(t, svc.RestoreBackup(t.Context(), "any"), ErrBackupInProgress)
	svc.backingUp = false
	svc.restoring = true
	_, err = svc.StartBackup(t.Context(), "manual", 0, "local")
	require.ErrorIs(t, err, ErrRestoreInProgress)
	_, err = svc.StartBackup(t.Context(), "manual", 0, "s3")
	require.ErrorIs(t, err, ErrRestoreInProgress)
	svc.restoring = false
}

func TestLocalZIPBackupRejectsSymlinkEscape(t *testing.T) {
	svc, _, directory := localBackupTestService(t)
	record, err := svc.startLocalBackup(t.Context(), "manual", 0, nil, false)
	require.NoError(t, err)
	outside := filepath.Join(t.TempDir(), "outside.zip")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0600))
	path := filepath.Join(directory, record.FileName)
	require.NoError(t, os.Remove(path))
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	file, _, err := svc.OpenLocalBackup(t.Context(), record.ID)
	if file != nil {
		_ = file.Close()
	}
	require.Error(t, err)
	require.NoError(t, svc.DeleteBackup(t.Context(), record.ID))
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
}
