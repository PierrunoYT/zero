//go:build windows

package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceBinaryReplacesRunningBinary(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "zero.exe")
	newPath := filepath.Join(dir, "zero.exe.new")

	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile new: %v", err)
	}

	if err := replaceBinary(targetPath, newPath); err != nil {
		t.Fatalf("replaceBinary: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(data) != "new-binary" {
		t.Fatalf("target content = %q, want %q", data, "new-binary")
	}
	if _, err := os.Stat(targetPath + ".old"); err != nil {
		t.Fatalf("expected the original binary to be preserved at %s.old: %v", targetPath, err)
	}
}

// When the install rename fails (newPath doesn't exist) but the restore
// succeeds, replaceBinary must put the original binary back at targetPath
// and report only the install failure, not a combined failure.
func TestReplaceBinaryRestoresOriginalWhenInstallFails(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "zero.exe")
	newPath := filepath.Join(dir, "zero.exe.new") // deliberately never created

	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}

	err := replaceBinary(targetPath, newPath)
	if err == nil {
		t.Fatal("expected replaceBinary to fail when newPath does not exist")
	}
	if strings.Contains(err.Error(), "additionally failed to restore") {
		t.Fatalf("expected the restore to succeed, got a combined failure: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target after restore: %v", err)
	}
	if string(data) != "old-binary" {
		t.Fatalf("target content after restore = %q, want %q", data, "old-binary")
	}
	if _, err := os.Stat(targetPath + ".old"); err == nil {
		t.Fatal("expected .old file to be gone after a successful restore")
	}
}

// If a previous replaceBinary call failed to both install and restore,
// targetPath is missing and oldPath is the only surviving copy of a working
// binary. CleanupStaleBinary must not delete it in that case.
func TestCleanupStaleBinaryPreservesOnlyRecoverableCopy(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "zero.exe") // never created: simulates a failed install+restore
	oldPath := targetPath + ".old"
	if err := os.WriteFile(oldPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile old: %v", err)
	}

	CleanupStaleBinary(targetPath)

	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("expected .old backup to survive when targetPath is missing: %v", err)
	}
}

// The normal case: targetPath exists (a previous replace succeeded), so the
// redundant .old backup should be cleaned up.
func TestCleanupStaleBinaryRemovesBackupWhenTargetExists(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "zero.exe")
	oldPath := targetPath + ".old"
	if err := os.WriteFile(targetPath, []byte("new-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	if err := os.WriteFile(oldPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile old: %v", err)
	}

	CleanupStaleBinary(targetPath)

	if _, err := os.Stat(oldPath); err == nil {
		t.Fatal("expected .old backup to be removed when targetPath exists")
	}
}

func TestRenameWithRetrySucceedsImmediately(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}

	if err := renameWithRetry(src, dst); err != nil {
		t.Fatalf("renameWithRetry: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("expected dst to exist after rename: %v", err)
	}
}

// A permanently-failing rename (source never appears) must exhaust its
// retries and surface the underlying error, rather than retrying forever.
func TestRenameWithRetryFailsAfterExhaustingAttempts(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	dst := filepath.Join(dir, "dst")

	if err := renameWithRetry(missing, dst); err == nil {
		t.Fatal("expected renameWithRetry to fail for a source that never appears")
	}
}
