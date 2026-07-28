package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gitlawb/zero/internal/daemon/remote"
	"github.com/Gitlawb/zero/internal/sandbox"
)

// daemonTokenFixture builds a workspace holding the remote bridge's token file
// alongside an ordinary file, with ZERO_DAEMON_REMOTE_TOKEN_FILE pointing at it —
// the shape a remote daemon session takes when its token lives in the session
// workspace. AllowRead deliberately covers the whole workspace so the tests prove
// the exclusion is not re-includable.
func daemonTokenFixture(t *testing.T) (string, string, *sandbox.Engine) {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws, "main.go"), []byte("package main // bridge-secret\n"), 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	token := filepath.Join(ws, "bridge-token")
	if err := os.WriteFile(token, []byte("bridge-secret\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	t.Setenv(remote.EnvToken, "")
	t.Setenv(remote.EnvTokenFile, token)

	scope, err := sandbox.NewScope(ws, nil)
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	engine := sandbox.NewEngine(sandbox.EngineOptions{
		WorkspaceRoot: ws,
		Policy: sandbox.Policy{
			Mode:             sandbox.ModeEnforce,
			EnforceWorkspace: true,
			AllowRead:        []string{ws},
		},
		Scope: scope,
	})
	return ws, token, engine
}

// defaultPolicyDaemonTokenEngine is the remote-daemon shape relevant to shell
// escalation: a selected token file with the default policy and no user
// DenyRead entries.
func defaultPolicyDaemonTokenEngine(t *testing.T) *sandbox.Engine {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	token := filepath.Join(ws, "bridge-token")
	if err := os.WriteFile(token, []byte("bridge-secret\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	t.Setenv(remote.EnvToken, "")
	t.Setenv(remote.EnvTokenFile, token)
	policy := sandbox.DefaultPolicy()
	if len(policy.DenyRead) != 0 {
		t.Fatalf("default policy DenyRead = %#v, want none", policy.DenyRead)
	}
	return sandbox.NewEngine(sandbox.EngineOptions{WorkspaceRoot: ws, Policy: policy})
}

func TestGrepSkipsDaemonTokenFile(t *testing.T) {
	ws, _, engine := daemonTokenFixture(t)
	tool, ok := NewScopedGrepTool(ws, nil).(sandboxAwareTool)
	if !ok {
		t.Fatal("grep tool must be sandbox-aware")
	}
	args := map[string]any{"pattern": "bridge-secret", "output_mode": "files_with_matches"}

	sandboxed := tool.RunWithSandbox(context.Background(), args, engine)
	if sandboxed.Status != StatusOK {
		t.Fatalf("grep failed: %s", sandboxed.Output)
	}
	if !strings.Contains(sandboxed.Output, "main.go") {
		t.Fatalf("grep must still match ordinary workspace files, got:\n%s", sandboxed.Output)
	}
	if strings.Contains(sandboxed.Output, "bridge-token") {
		t.Fatalf("grep must NOT surface the remote bridge token file, got:\n%s", sandboxed.Output)
	}
}

func TestGlobSkipsDaemonTokenFile(t *testing.T) {
	ws, _, engine := daemonTokenFixture(t)
	tool, ok := NewScopedGlobTool(ws, nil).(sandboxAwareTool)
	if !ok {
		t.Fatal("glob tool must be sandbox-aware")
	}

	sandboxed := tool.RunWithSandbox(context.Background(), map[string]any{"pattern": "**/*"}, engine)
	if sandboxed.Status != StatusOK {
		t.Fatalf("glob failed: %s", sandboxed.Output)
	}
	if !strings.Contains(sandboxed.Output, "main.go") {
		t.Fatalf("glob must still match ordinary workspace files, got:\n%s", sandboxed.Output)
	}
	if strings.Contains(sandboxed.Output, "bridge-token") {
		t.Fatalf("glob must NOT surface the remote bridge token file, got:\n%s", sandboxed.Output)
	}
}

func TestListDirectorySkipsDaemonTokenFile(t *testing.T) {
	ws, _, engine := daemonTokenFixture(t)
	registry := NewRegistry()
	registry.Register(NewScopedListDirectoryTool(ws, nil))

	result := registry.RunWithOptions(context.Background(), "list_directory", map[string]any{
		"path": ".",
	}, RunOptions{Sandbox: engine})
	if result.Status != StatusOK {
		t.Fatalf("list_directory failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "main.go") {
		t.Fatalf("list_directory must still show ordinary workspace files, got:\n%s", result.Output)
	}
	if strings.Contains(result.Output, "bridge-token") {
		t.Fatalf("list_directory must NOT surface the remote bridge token file, got:\n%s", result.Output)
	}
}

// TestEngineDeniesDaemonTokenFileTools covers the request gate the direct file
// tools go through (read_file, write_file, edit_file, apply_patch): the bridge
// token must be neither readable nor writable, even though AllowRead covers the
// whole workspace and the file is inside it.
func TestEngineDeniesDaemonTokenFileTools(t *testing.T) {
	ws, token, engine := daemonTokenFixture(t)
	for _, tc := range []struct {
		name       string
		toolName   string
		sideEffect sandbox.SideEffect
	}{
		{name: "read_file", toolName: "read_file", sideEffect: sandbox.SideEffectRead},
		{name: "write_file", toolName: "write_file", sideEffect: sandbox.SideEffectWrite},
		{name: "edit_file", toolName: "edit_file", sideEffect: sandbox.SideEffectWrite},
		{name: "apply_patch", toolName: "apply_patch", sideEffect: sandbox.SideEffectWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := engine.Evaluate(context.Background(), sandbox.Request{
				ToolName:      tc.toolName,
				WorkspaceRoot: ws,
				SideEffect:    tc.sideEffect,
				Args:          map[string]any{"path": token},
				// A granted permission must not override the exclusion either.
				Permission: sandbox.PermissionAllow,
			})
			if decision.Action != sandbox.ActionDeny || !strings.Contains(decision.Reason, "remote bridge token") {
				t.Fatalf("%s on the bridge token: action = %q reason = %q, want a bridge-token deny", tc.toolName, decision.Action, decision.Reason)
			}
		})
	}

	// An ordinary workspace file in the same directory stays usable.
	decision := engine.Evaluate(context.Background(), sandbox.Request{
		ToolName:      "read_file",
		WorkspaceRoot: ws,
		SideEffect:    sandbox.SideEffectRead,
		Args:          map[string]any{"path": filepath.Join(ws, "main.go")},
	})
	if decision.Action == sandbox.ActionDeny {
		t.Fatalf("ordinary workspace read was denied: %q", decision.Reason)
	}
}

func TestDaemonTokenAliasesDeniedEndToEnd(t *testing.T) {
	for _, aliasKind := range []string{"symlink", "hardlink"} {
		t.Run(aliasKind, func(t *testing.T) {
			ws, token, engine := daemonTokenFixture(t)
			alias := filepath.Join(ws, "token-alias")
			var err error
			switch aliasKind {
			case "symlink":
				err = os.Symlink(token, alias)
			case "hardlink":
				err = os.Link(token, alias)
			}
			if err != nil {
				t.Skipf("%s unsupported: %v", aliasKind, err)
			}

			registry := NewRegistry()
			registry.Register(NewScopedReadFileTool(ws, nil))
			registry.Register(NewScopedWriteFileTool(ws, nil))

			read := registry.RunWithOptions(context.Background(), "read_file", map[string]any{"path": alias}, RunOptions{Sandbox: engine})
			if read.Status == StatusOK || strings.Contains(read.Output, "bridge-secret") {
				t.Fatalf("read_file followed protected %s: status=%s output=%q", aliasKind, read.Status, read.Output)
			}

			write := registry.RunWithOptions(context.Background(), "write_file", map[string]any{"path": alias, "content": "attacker-controlled\n"}, RunOptions{Sandbox: engine})
			if write.Status == StatusOK {
				t.Fatalf("write_file followed protected %s: output=%q", aliasKind, write.Output)
			}
			contents, err := os.ReadFile(token)
			if err != nil || string(contents) != "bridge-secret\n" {
				t.Fatalf("token changed after denied write through %s: contents=%q err=%v", aliasKind, contents, err)
			}

			grep, ok := NewScopedGrepTool(ws, nil).(sandboxAwareTool)
			if !ok {
				t.Fatal("grep tool must be sandbox-aware")
			}
			result := grep.RunWithSandbox(context.Background(), map[string]any{
				"pattern":     "bridge-secret",
				"output_mode": "files_with_matches",
			}, engine)
			if result.Status != StatusOK {
				t.Fatalf("grep failed: %s", result.Output)
			}
			if strings.Contains(result.Output, "token-alias") || strings.Contains(result.Output, "bridge-token") {
				t.Fatalf("grep surfaced protected %s target:\n%s", aliasKind, result.Output)
			}
			if !strings.Contains(result.Output, "main.go") {
				t.Fatalf("grep omitted ordinary file while filtering %s:\n%s", aliasKind, result.Output)
			}
		})
	}
}
