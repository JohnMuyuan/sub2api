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
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

const settingKeyBackupLocalConfig = "backup_local_config"

type BackupLocalConfig struct {
	Directory string `json:"directory"`
}

func (s *BackupService) GetLocalConfig(ctx context.Context) (*BackupLocalConfig, error) {
	value, err := s.settingRepo.GetValue(ctx, settingKeyBackupLocalConfig)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, err
	}
	cfg := &BackupLocalConfig{}
	if value != "" {
		if err := json.Unmarshal([]byte(value), cfg); err != nil {
			return nil, fmt.Errorf("read local backup configuration: %w", err)
		}
	}
	return cfg, nil
}

func validateLocalBackupDirectory(directory string) error {
	if directory == "" || !filepath.IsAbs(directory) || strings.ContainsRune(directory, 0) {
		return infraerrors.BadRequest("INVALID_BACKUP_DIRECTORY", "local backup directory must be an absolute path")
	}
	return nil
}

func (s *BackupService) UpdateLocalConfig(ctx context.Context, cfg BackupLocalConfig) (*BackupLocalConfig, error) {
	cfg.Directory = strings.TrimSpace(cfg.Directory)
	if err := validateLocalBackupDirectory(cfg.Directory); err != nil {
		return nil, err
	}
	cfg.Directory = filepath.Clean(cfg.Directory)
	root, err := openLocalBackupDirectory(cfg.Directory)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	// Verify writability without truncating any existing file.
	name := ".sub2api-write-test-" + uuid.NewString()
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("backup directory is not writable: %w", err)
	}
	closeErr := file.Close()
	removeErr := root.Remove(name)
	if err := errors.Join(closeErr, removeErr); err != nil {
		return nil, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, settingKeyBackupLocalConfig, string(data)); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func openLocalBackupDirectory(directory string) (*os.Root, error) {
	if err := validateLocalBackupDirectory(directory); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0700); err != nil { // #nosec G703 -- Explicit administrator-selected absolute directory, validated above; child paths are confined by os.Root.
		return nil, fmt.Errorf("create backup directory: %w", err)
	}
	return os.OpenRoot(directory)
}

// Both manual and scheduled jobs share the existing operation guard and records.
func (s *BackupService) startLocalBackup(ctx context.Context, triggeredBy string, expireDays int, schedule *BackupScheduleConfig, async bool) (*BackupRecord, error) {
	s.opMu.Lock()
	if s.shuttingDown.Load() {
		s.opMu.Unlock()
		return nil, infraerrors.ServiceUnavailable("SERVER_SHUTTING_DOWN", "server is shutting down")
	}
	if s.backingUp {
		s.opMu.Unlock()
		return nil, ErrBackupInProgress
	}
	if s.restoring {
		s.opMu.Unlock()
		return nil, ErrRestoreInProgress
	}
	s.backingUp = true
	s.wg.Add(1)
	s.opMu.Unlock()
	finish := func() {
		s.opMu.Lock()
		s.backingUp = false
		s.opMu.Unlock()
		s.wg.Done()
	}
	cfg, err := s.GetLocalConfig(ctx)
	if err != nil {
		finish()
		return nil, err
	}
	if err := validateLocalBackupDirectory(cfg.Directory); err != nil {
		finish()
		return nil, err
	}
	now := time.Now()
	id := uuid.NewString()
	record := &BackupRecord{
		ID: id, Status: "running", BackupType: "postgres", Storage: "local",
		LocalDirectory: cfg.Directory, FileName: "sub2api_" + now.Format("20060102_150405") + "_" + id + ".zip",
		Portable:    true,
		TriggeredBy: triggeredBy, StartedAt: now.Format(time.RFC3339), Progress: "dumping",
	}
	if expireDays > 0 {
		record.ExpiresAt = now.AddDate(0, 0, expireDays).Format(time.RFC3339)
	}
	if err := s.saveRecord(ctx, record); err != nil {
		finish()
		return nil, err
	}
	result := *record
	run := func(parent context.Context) (runErr error) {
		defer finish()
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = fmt.Errorf("local backup panic: %v", recovered)
				record.Status, record.ErrorMsg, record.Progress = "failed", runErr.Error(), ""
				record.FinishedAt = time.Now().Format(time.RFC3339)
				saveCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
				defer cancel()
				runErr = errors.Join(runErr, s.saveRecord(saveCtx, record))
			}
		}()
		ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
		defer cancel()
		size, err := s.writeLocalZIP(ctx, record)
		record.SizeBytes, record.Progress = size, ""
		record.FinishedAt = time.Now().Format(time.RFC3339)
		record.Status = "completed"
		if err != nil {
			record.Status, record.ErrorMsg = "failed", err.Error()
		}
		saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
		defer saveCancel()
		return errors.Join(err, s.saveRecordWithArchive(saveCtx, record, schedule))
	}
	if !async {
		err := run(ctx)
		return record, err
	}
	go func() {
		if err := run(s.bgCtx); err != nil {
			logger.LegacyPrintf("service.backup", "[Backup] local ZIP backup failed: %v", err)
		}
	}()
	return &result, nil
}

func (s *BackupService) writeLocalZIP(ctx context.Context, record *BackupRecord) (size int64, err error) {
	root, err := openLocalBackupDirectory(record.LocalDirectory)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	tempName := "." + record.FileName + ".tmp"
	file, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return 0, err
	}
	published := false
	defer func() {
		_ = file.Close()
		if !published {
			_ = root.Remove(tempName)
		}
	}()
	dump, err := s.dumper.Dump(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = dump.Close() }()
	archive := zip.NewWriter(file)
	entry, err := archive.Create("database.sql")
	digest := sha256.New()
	if err == nil {
		var written int64
		written, err = io.Copy(io.MultiWriter(entry, digest), io.LimitReader(dump, maxBackupSQLBytes+1))
		if err == nil && written > maxBackupSQLBytes {
			err = errors.New("database.sql exceeds the supported 8 GiB ZIP restore limit")
		}
	}
	if err == nil {
		var metadata io.Writer
		metadata, err = archive.Create("manifest.json")
		if err == nil {
			err = json.NewEncoder(metadata).Encode(localZIPManifest{Format: "sub2api", Version: 1, SQLSHA256: hex.EncodeToString(digest.Sum(nil)), KeysInDatabase: true})
		}
	}
	err = errors.Join(err, dump.Close(), archive.Close())
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if err := file.Close(); err != nil {
		return 0, err
	}
	if err := root.Rename(tempName, record.FileName); err != nil {
		return 0, err
	}
	published = true
	return info.Size(), nil
}

func localBackupRoot(record *BackupRecord) (*os.Root, error) {
	if record.Storage != "local" || filepath.Base(record.FileName) != record.FileName || !strings.HasPrefix(record.FileName, "sub2api_") || !strings.HasSuffix(record.FileName, "_"+record.ID+".zip") || strings.ContainsAny(record.ID, `/\`) {
		return nil, errors.New("invalid local backup file metadata")
	}
	if err := validateLocalBackupDirectory(record.LocalDirectory); err != nil {
		return nil, err
	}
	return os.OpenRoot(record.LocalDirectory)
}

func (s *BackupService) OpenLocalBackup(ctx context.Context, id string) (*os.File, *BackupRecord, error) {
	record, err := s.GetBackupRecord(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if record.Status != "completed" {
		return nil, nil, infraerrors.BadRequest("BACKUP_NOT_COMPLETED", "backup is not completed")
	}
	root, err := localBackupRoot(record)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(record.FileName)
	return file, record, err
}

func deleteLocalBackup(record *BackupRecord) error {
	root, err := localBackupRoot(record)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, name := range []string{record.FileName, "." + record.FileName + ".tmp"} {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *BackupService) restoreLocalZIP(ctx context.Context, record *BackupRecord) error {
	root, err := localBackupRoot(record)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(record.FileName)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	reader, _, err := validateLocalZIP(ctx, file, info.Size())
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close(); _ = os.Remove(reader.Name()) }()
	return s.dumper.Restore(ctx, reader)
}

func (s *BackupService) restoreObjectStore(ctx context.Context, record *BackupRecord) (BackupObjectStore, error) {
	if record.Storage == "local" {
		file, _, err := s.OpenLocalBackup(ctx, record.ID)
		if err != nil {
			return nil, err
		}
		return nil, file.Close()
	}
	cfg, err := s.loadS3Config(ctx)
	if err != nil {
		return nil, err
	}
	return s.getOrCreateStore(ctx, cfg)
}
