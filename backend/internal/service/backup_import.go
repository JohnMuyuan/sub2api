package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const MaxBackupUploadBytes int64 = 256 << 20
const maxBackupSQLBytes int64 = 8 << 30

type localZIPManifest struct {
	Format         string `json:"format"`
	Version        int    `json:"version"`
	SQLSHA256      string `json:"sql_sha256"`
	KeysInDatabase bool   `json:"keys_in_database"`
}

// Validate and fully materialize SQL before invoking psql: a corrupt ZIP must
// never be discovered only after PostgreSQL has committed a partial input.
func validateLocalZIP(ctx context.Context, source io.ReaderAt, size int64) (sql *os.File, portable bool, err error) {
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return nil, false, err
	}
	var database, manifest *zip.File
	for _, entry := range archive.File {
		if !entry.Mode().IsRegular() {
			return nil, false, errors.New("backup contains a non-regular file")
		}
		switch entry.Name {
		case "database.sql":
			if database != nil {
				return nil, false, errors.New("duplicate database.sql")
			}
			database = entry
		case "manifest.json":
			if manifest != nil {
				return nil, false, errors.New("duplicate manifest.json")
			}
			manifest = entry
		default:
			return nil, false, fmt.Errorf("unsupported backup entry: %s", entry.Name)
		}
	}
	if database == nil || database.UncompressedSize64 == 0 || database.UncompressedSize64 > uint64(maxBackupSQLBytes) {
		return nil, false, errors.New("backup must contain a non-empty database.sql no larger than 8 GiB")
	}
	var meta localZIPManifest
	if manifest != nil {
		if manifest.UncompressedSize64 > 4096 {
			return nil, false, errors.New("backup manifest is too large")
		}
		reader, openErr := manifest.Open()
		if openErr != nil {
			return nil, false, openErr
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, 4097))
		closeErr := reader.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, false, err
		}
		if len(data) > 4096 {
			return nil, false, errors.New("backup manifest is too large")
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, false, err
		}
		digest, decodeErr := hex.DecodeString(meta.SQLSHA256)
		if meta.Format != "sub2api" || meta.Version != 1 || decodeErr != nil || len(digest) != sha256.Size {
			return nil, false, errors.New("unsupported or invalid backup manifest")
		}
	}
	reader, err := database.Open()
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = reader.Close() }()
	sql, err = os.CreateTemp("", "sub2api-restore-*.sql")
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if err != nil {
			_ = sql.Close()
			_ = os.Remove(sql.Name())
		}
	}()
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(sql, digest), io.LimitReader(&backupContextReader{ctx: ctx, reader: reader}, maxBackupSQLBytes+1))
	if err != nil {
		return sql, false, err
	}
	if written > maxBackupSQLBytes {
		return sql, false, errors.New("database.sql exceeds 8 GiB")
	}
	if meta.SQLSHA256 != "" && !strings.EqualFold(meta.SQLSHA256, hex.EncodeToString(digest.Sum(nil))) {
		return sql, false, errors.New("database.sql checksum mismatch")
	}
	if _, err = sql.Seek(0, io.SeekStart); err != nil {
		return sql, false, err
	}
	return sql, meta.KeysInDatabase, nil
}

type backupContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *backupContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Import only registers a validated backup. Destructive restore still goes
// through the existing administrator-password and operation-lock flow.
func (s *BackupService) ImportLocalZIP(ctx context.Context, upload io.Reader) (_ *BackupRecord, err error) {
	dir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dir == "" {
		if info, statErr := os.Stat("/app/data"); statErr == nil && info.IsDir() {
			dir = "/app/data"
		} else {
			dir = "./data"
		}
	}
	dir, err = filepath.Abs(filepath.Join(dir, "backup-imports"))
	if err != nil {
		return nil, err
	}
	root, err := openLocalBackupDirectory(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	id := uuid.NewString()
	name := "sub2api_import_" + id + ".zip"
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = root.Remove(name)
		}
	}()
	size, err := io.Copy(file, io.LimitReader(&backupContextReader{ctx: ctx, reader: upload}, MaxBackupUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if size > MaxBackupUploadBytes {
		return nil, infraerrors.BadRequest("BACKUP_UPLOAD_TOO_LARGE", "backup upload exceeds 256 MiB")
	}
	sql, portable, err := validateLocalZIP(ctx, file, size)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_BACKUP_ZIP", "invalid backup ZIP: "+err.Error())
	}
	_ = sql.Close()
	_ = os.Remove(sql.Name()) // #nosec G703 -- This filename comes only from os.CreateTemp, never from a ZIP entry.
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	now := time.Now()
	record := &BackupRecord{ID: id, Status: "completed", BackupType: "postgres", Storage: "local", LocalDirectory: dir,
		FileName: name, SizeBytes: size, TriggeredBy: "import", Portable: portable,
		StartedAt: now.Format(time.RFC3339), FinishedAt: now.Format(time.RFC3339), ExpiresAt: now.AddDate(0, 0, 7).Format(time.RFC3339)}
	if err := s.saveRecord(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}
