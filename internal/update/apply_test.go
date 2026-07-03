package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Gitlawb/zero/internal/release"
)

func TestApplyReturnsNoopWhenUpToDate(t *testing.T) {
	result, err := Apply(context.Background(), Options{
		CurrentVersion: "0.2.0",
		GOOS:           "linux",
		GOARCH:         "amd64",
		Fetch: func(_ context.Context, endpoint string) (Release, error) {
			return releaseForTarget(t, "v0.2.0", "linux", "amd64"), nil
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Applied {
		t.Fatalf("expected Applied=false when already up to date, got %#v", result)
	}
}

func TestApplyStandaloneUpdateReplacesBinary(t *testing.T) {
	binaryName := "zero"
	optionalName := "zero-seccomp"
	if runtime.GOOS == "windows" {
		binaryName = "zero.exe"
		optionalName = "zero-windows-command-runner.exe"
	}

	installDir := t.TempDir()
	executablePath := filepath.Join(installDir, binaryName)
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile executable: %v", err)
	}
	// Only pre-existing optional helpers should be refreshed; an absent one
	// (e.g. the platform's other optional helper) must not be introduced.
	existingHelperPath := filepath.Join(installDir, optionalName)
	if err := os.WriteFile(existingHelperPath, []byte("old-helper"), 0o755); err != nil {
		t.Fatalf("WriteFile helper: %v", err)
	}

	archiveName := "zero-v0.2.0-linux-x64.tar.gz"
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, archiveName)
	writeTestTarGz(t, archivePath, map[string]string{
		"zero":                            "new-binary",
		"zero.exe":                        "new-binary-exe",
		"zero-seccomp":                    "new-helper",
		"zero-windows-command-runner.exe": "new-helper-exe",
	})
	checksum, err := release.SHA256File(archivePath)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	checksumText, err := release.FormatSHA256Checksum(checksum, archiveName)
	if err != nil {
		t.Fatalf("FormatSHA256Checksum: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + archiveName:
			http.ServeFile(w, r, archivePath)
		case "/" + archiveName + ".sha256":
			_, _ = w.Write([]byte(checksumText))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := Result{
		LatestVersion: "0.2.0",
		ReleaseAsset: AssetCheck{
			Platform:      "linux",
			Arch:          "x64",
			ArchiveName:   archiveName,
			ArchiveURL:    server.URL + "/" + archiveName,
			ChecksumName:  archiveName + ".sha256",
			ChecksumURL:   server.URL + "/" + archiveName + ".sha256",
			ArchiveFound:  true,
			ChecksumFound: true,
			Verified:      true,
		},
	}

	if err := applyStandaloneUpdate(context.Background(), result, executablePath); err != nil {
		t.Fatalf("applyStandaloneUpdate returned error: %v", err)
	}

	data, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable: %v", err)
	}
	wantBinary := "new-binary"
	if runtime.GOOS == "windows" {
		wantBinary = "new-binary-exe"
	}
	if string(data) != wantBinary {
		t.Fatalf("executable content = %q, want %q", data, wantBinary)
	}

	helperData, err := os.ReadFile(existingHelperPath)
	if err != nil {
		t.Fatalf("ReadFile helper: %v", err)
	}
	wantHelper := "new-helper"
	if runtime.GOOS == "windows" {
		wantHelper = "new-helper-exe"
	}
	if string(helperData) != wantHelper {
		t.Fatalf("helper content = %q, want %q", helperData, wantHelper)
	}

	if entries, err := os.ReadDir(installDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			// On Windows, replaceBinary leaves "<name>.old" behind (the running
			// binary is renamed aside, not deleted) for later best-effort cleanup.
			if name == binaryName || name == optionalName || name == binaryName+".old" || name == optionalName+".old" {
				continue
			}
			t.Fatalf("unexpected extra file left in install dir: %s", name)
		}
	}
}

func TestApplyStandaloneUpdateRejectsChecksumMismatch(t *testing.T) {
	binaryName := "zero"
	if runtime.GOOS == "windows" {
		binaryName = "zero.exe"
	}

	installDir := t.TempDir()
	executablePath := filepath.Join(installDir, binaryName)
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile executable: %v", err)
	}

	archiveName := "zero-v0.2.0-linux-x64.tar.gz"
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, archiveName)
	writeTestTarGz(t, archivePath, map[string]string{"zero": "new-binary", "zero.exe": "new-binary-exe"})

	badChecksumText, err := release.FormatSHA256Checksum("0000000000000000000000000000000000000000000000000000000000000000"[:64], archiveName)
	if err != nil {
		t.Fatalf("FormatSHA256Checksum: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + archiveName:
			http.ServeFile(w, r, archivePath)
		case "/" + archiveName + ".sha256":
			_, _ = w.Write([]byte(badChecksumText))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result := Result{
		ReleaseAsset: AssetCheck{
			Platform:      "linux",
			Arch:          "x64",
			ArchiveName:   archiveName,
			ArchiveURL:    server.URL + "/" + archiveName,
			ChecksumName:  archiveName + ".sha256",
			ChecksumURL:   server.URL + "/" + archiveName + ".sha256",
			ArchiveFound:  true,
			ChecksumFound: true,
			Verified:      true,
		},
	}

	if err := applyStandaloneUpdate(context.Background(), result, executablePath); err == nil {
		t.Fatal("expected checksum mismatch error")
	}

	data, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable: %v", err)
	}
	if string(data) != "old-binary" {
		t.Fatalf("executable should be untouched after checksum failure, got %q", data)
	}
}
