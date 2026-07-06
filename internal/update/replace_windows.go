//go:build windows

package update

import (
	"fmt"
	"os"
	"time"
)

const (
	renameRetryAttempts = 10
	renameRetryDelay    = 100 * time.Millisecond
)

// replaceBinary installs newPath over targetPath. Windows will not let a
// running executable be overwritten or deleted directly, but NTFS does allow
// renaming it aside — the same trick already used for locked config files in
// internal/cli/mcp_config.go's replaceMCPWritableConfigFile.
func replaceBinary(targetPath string, newPath string) error {
	oldPath := targetPath + ".old"
	_ = os.Remove(oldPath) // best-effort cleanup of a leftover from a previous upgrade
	if err := os.Rename(targetPath, oldPath); err != nil {
		return fmt.Errorf("rename running binary aside: %w", err)
	}
	// Retry both the install and the restore: a transient Windows file lock
	// (antivirus/indexer scanning the just-renamed file, a lingering handle)
	// can make either rename fail momentarily, and on the restore side
	// failure means targetPath is left missing entirely rather than merely
	// stale — worth a short retry to avoid that.
	if err := renameWithRetry(newPath, targetPath); err != nil {
		if restoreErr := renameWithRetry(oldPath, targetPath); restoreErr != nil {
			return fmt.Errorf("install new binary: %w; additionally failed to restore the original binary: %v (original preserved at %s)", err, restoreErr, oldPath)
		}
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}

func renameWithRetry(oldPath string, newPath string) error {
	var lastErr error
	for attempt := 0; attempt < renameRetryAttempts; attempt++ {
		lastErr = os.Rename(oldPath, newPath)
		if lastErr == nil {
			return nil
		}
		if attempt < renameRetryAttempts-1 {
			time.Sleep(renameRetryDelay)
		}
	}
	return lastErr
}

// CleanupStaleBinary best-effort removes a "<path>.old" file left behind by a
// previous replaceBinary call once the old process holding it has exited.
// Callers should invoke this once at startup for the current executable.
//
// Guarded on targetPath still existing: if a previous replaceBinary call
// failed to install AND failed to restore the original, targetPath is
// missing and oldPath is the only surviving copy of a working binary --
// removing it here would destroy the last recoverable copy instead of just
// clearing redundant backup left by a successful replace.
func CleanupStaleBinary(targetPath string) {
	if _, err := os.Stat(targetPath); err != nil {
		return
	}
	_ = os.Remove(targetPath + ".old")
}
