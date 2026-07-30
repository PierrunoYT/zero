package cli

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolateDaemonPaths points DefaultPaths at a temp dir so the test never touches
// a real daemon on the dev machine.
func isolateDaemonPaths(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
}

func runDaemonCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runDaemon(args, &out, &errb, appDeps{})
	return code, out.String(), errb.String()
}

func TestDaemonUsage(t *testing.T) {
	code, _, _ := runDaemonCLI(t)
	if code != exitUsage {
		t.Fatalf("no-args exit = %d, want exitUsage", code)
	}
	code, out, _ := runDaemonCLI(t, "--help")
	if code != exitSuccess || !strings.Contains(out, "Usage: zero daemon") {
		t.Fatalf("--help exit=%d out=%q", code, out)
	}
}

func TestDaemonUnknownSubcommand(t *testing.T) {
	code, _, errb := runDaemonCLI(t, "frobnicate")
	if code != exitUsage {
		t.Fatalf("unknown subcommand exit = %d, want exitUsage", code)
	}
	if !strings.Contains(errb, "unknown daemon subcommand") {
		t.Fatalf("stderr = %q, want unknown-subcommand message", errb)
	}
}

func TestDaemonRunRequiresSession(t *testing.T) {
	isolateDaemonPaths(t)
	code, _, errb := runDaemonCLI(t, "run", "--prompt", "hi")
	if code != exitUsage {
		t.Fatalf("run without --session exit = %d, want exitUsage", code)
	}
	if !strings.Contains(errb, "--session") {
		t.Fatalf("stderr = %q, want a --session hint", errb)
	}
}

func TestDaemonRunRequiresPromptOrArgs(t *testing.T) {
	isolateDaemonPaths(t)
	code, _, errb := runDaemonCLI(t, "run", "--session", "s1")
	if code != exitUsage {
		t.Fatalf("run without prompt/args exit = %d, want exitUsage", code)
	}
	if !strings.Contains(errb, "--prompt") {
		t.Fatalf("stderr = %q, want a --prompt hint", errb)
	}
}

func TestDaemonStopWhenNotRunning(t *testing.T) {
	isolateDaemonPaths(t)
	code, out, _ := runDaemonCLI(t, "stop")
	if code != exitSuccess {
		t.Fatalf("stop (not running) exit = %d, want exitSuccess", code)
	}
	if !strings.Contains(out, "not running") {
		t.Fatalf("stop output = %q, want 'not running'", out)
	}
}

func TestDaemonStatusWhenNotRunning(t *testing.T) {
	isolateDaemonPaths(t)
	code, out, _ := runDaemonCLI(t, "status")
	if code != exitSuccess {
		t.Fatalf("status (not running) exit = %d, want exitSuccess", code)
	}
	if !strings.Contains(out, "not running") {
		t.Fatalf("status output = %q, want 'not running'", out)
	}
}

func TestDaemonAttachRequiresSession(t *testing.T) {
	isolateDaemonPaths(t)
	code, _, errb := runDaemonCLI(t, "attach")
	if code != exitUsage {
		t.Fatalf("attach without session exit = %d, want exitUsage", code)
	}
	if !strings.Contains(errb, "session") {
		t.Fatalf("stderr = %q, want a session hint", errb)
	}
}

func TestDaemonRunWhenNotRunning(t *testing.T) {
	isolateDaemonPaths(t)
	code, _, errb := runDaemonCLI(t, "run", "--session", "s1", "--prompt", "hello")
	if code != exitCrash {
		t.Fatalf("run (no daemon) exit = %d, want exitCrash", code)
	}
	if !strings.Contains(errb, "not running") {
		t.Fatalf("stderr = %q, want 'not running'", errb)
	}
}

func TestDaemonSubcommandsRejectExtraArgs(t *testing.T) {
	isolateDaemonPaths(t)
	cases := [][]string{
		{"stop", "oops"},
		{"status", "oops"},
		{"attach", "s1", "extra"},
	}
	for _, args := range cases {
		code, _, errb := runDaemonCLI(t, args...)
		if code != exitUsage {
			t.Fatalf("%v exit = %d, want exitUsage (reject extra args); stderr=%q", args, code, errb)
		}
	}
}

func TestDaemonServeRemoteCanonicalizesTokenFileBeforeStartingWorkers(t *testing.T) {
	isolateDaemonPaths(t)
	certFile, keyFile := writeDaemonTestCertificate(t)
	startDir := t.TempDir()
	t.Chdir(startDir)
	if err := os.WriteFile("token", []byte("bridge-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZERO_DAEMON_REMOTE_TOKEN", "")
	t.Setenv("ZERO_DAEMON_REMOTE_TOKEN_FILE", "token")

	code, _, _ := runDaemonCLI(t, "serve-remote", "--addr", "127.0.0.1:not-a-port", "--tls-cert", certFile, "--tls-key", keyFile)
	if code != exitCrash {
		t.Fatalf("serve-remote exit = %d, want bind failure", code)
	}
	want := filepath.Join(startDir, "token")
	want, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", want, err)
	}
	if got := os.Getenv("ZERO_DAEMON_REMOTE_TOKEN_FILE"); got != want {
		t.Fatalf("ZERO_DAEMON_REMOTE_TOKEN_FILE = %q, want daemon-pinned path %q", got, want)
	}
}

func writeDaemonTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "zero-daemon-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	if err := certOut.Close(); err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatal(err)
	}
	if err := keyOut.Close(); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
