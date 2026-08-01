package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Gitlawb/zero/internal/sandbox"
)

type applyPatchTool struct {
	baseTool
	workspaceRoot string
	scope         PathScope
}

func NewScopedApplyPatchTool(workspaceRoot string, scope PathScope) Tool {
	return applyPatchTool{
		baseTool: baseTool{
			name:        "apply_patch",
			description: "Apply a unified diff patch inside the workspace or an explicitly granted extra write root.",
			parameters: Schema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"patch": {Type: "string", Description: "Unified diff patch to apply."},
					"cwd":   {Type: "string", Description: "Directory where the patch should be applied. Relative paths stay in the workspace; use an absolute path to target a granted extra write root. Defaults to workspace root.", Default: "."},
				},
				Required:             []string{"patch"},
				AdditionalProperties: false,
			},
			safety:       promptSafety(SideEffectWrite, "Applies patch hunks that can create, edit, or delete files."),
			capabilities: ToolCapabilities{Effect: EffectWorkspaceWrite, ThreadSafe: false, ResourceKeys: applyPatchResourceKeys},
		},
		workspaceRoot: normalizeWorkspaceRoot(workspaceRoot),
		scope:         scope,
	}
}

func (tool applyPatchTool) Run(ctx context.Context, args map[string]any) Result {
	return tool.RunWithOptions(ctx, args, RunOptions{})
}

func (tool applyPatchTool) RunWithOptions(ctx context.Context, args map[string]any, options RunOptions) Result {
	patch, err := aliasedStringArg(args, []string{"patch", "diff"}, "", true, false)
	if err != nil {
		return errorResult("Error: Invalid arguments for apply_patch: " + err.Error())
	}
	cwd, err := stringArg(args, "cwd", ".", false)
	if err != nil {
		return errorResult("Error: Invalid arguments for apply_patch: " + err.Error())
	}

	applyRoot, relativeRoot, err := resolveScopedPath(tool.workspaceRoot, tool.scope, cwd)
	if err != nil {
		return errorResult("Error applying patch: " + err.Error())
	}
	if err := validatePatchPaths(applyRoot, patch); err != nil {
		return errorResult("Error applying patch: " + err.Error())
	}

	tempFile, err := os.CreateTemp("", "zero-patch-*.patch")
	if err != nil {
		return errorResult("Error applying patch: " + err.Error())
	}
	patchPath := tempFile.Name()
	defer func() {
		_ = os.Remove(patchPath)
	}()
	if _, err := tempFile.WriteString(patch); err != nil {
		_ = tempFile.Close()
		return errorResult("Error applying patch: " + err.Error())
	}
	if err := tempFile.Close(); err != nil {
		return errorResult("Error applying patch: " + err.Error())
	}

	if err := recheckPatchWriteTargets(applyRoot, patch); err != nil {
		return errorResult("Error applying patch: " + err.Error())
	}
	var createdTargets []string
	if options.FileTracker != nil {
		createdTargets = missingPatchTargets(applyRoot, patch)
	}

	command := exec.CommandContext(ctx, "git", "apply", "--whitespace=nowarn", patchPath)
	command.Dir = applyRoot
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return errorResult("Error applying patch: " + message)
	}

	summary := "Patch applied successfully."
	if relativeRoot != "." {
		summary = "Patch applied successfully in " + relativeRoot + "."
	}
	result := okResult(summary)
	result.ChangedFiles = changedFilesFromPatch(relativeRoot, patch)
	result.Display = Display{Summary: summary, Kind: "diff", Preview: capPreviewDiff(patch)}
	// git apply already rejects a patch whose context drifted, so it has its own
	// staleness guard. Drop any tracked baseline for the files it rewrote so a
	// subsequent edit_file/write_file re-reads instead of false-flagging the
	// patch's own change as an external modification.
	for _, changed := range result.ChangedFiles {
		if absolute, _, rerr := resolveScopedPath(tool.workspaceRoot, tool.scope, changed); rerr == nil {
			options.FileTracker.Forget(absolute)
		}
	}
	recordCreatedPatchTargets(options.FileTracker, createdTargets)
	return result
}

func missingPatchTargets(root string, patch string) []string {
	seen := map[string]bool{}
	var missing []string
	for _, path := range sandbox.PatchHeaderPaths(patch) {
		if path == "" || path == "/dev/null" {
			continue
		}
		absolute, _, err := resolveWorkspaceTargetPath(root, path)
		if err != nil || seen[absolute] {
			continue
		}
		seen[absolute] = true
		if _, err := os.Stat(absolute); os.IsNotExist(err) {
			missing = append(missing, absolute)
		}
	}
	return missing
}

func recordCreatedPatchTargets(tracker *FileTracker, missingBefore []string) {
	if tracker == nil {
		return
	}
	for _, absolute := range missingBefore {
		if _, err := os.Stat(absolute); err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = resolved
		}
		tracker.RecordCreated(absolute)
	}
}

// changedFilesFromPatch extracts the unique, WORKSPACE-relative paths a patch
// touches, reusing the same per-line parser used for validation. Patch paths are
// relative to the apply cwd, so relativeRoot (the workspace-relative cwd, e.g.
// "sub/dir", or "." for the workspace root) is prefixed so callers get true
// workspace-relative paths regardless of cwd. When the apply cwd resolves to an
// extra write root, resolveScopedPath returns the absolute path as relativeRoot;
// in that case the entries in the returned slice are absolute paths, since
// workspace-relative would be ambiguous there.
func changedFilesFromPatch(relativeRoot string, patch string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, path := range sandbox.PatchHeaderPaths(patch) {
		if path == "" || path == "/dev/null" {
			continue
		}
		workspacePath := path
		if relativeRoot != "" && relativeRoot != "." {
			workspacePath = filepath.ToSlash(filepath.Join(relativeRoot, path))
		}
		if seen[workspacePath] {
			continue
		}
		seen[workspacePath] = true
		paths = append(paths, workspacePath)
	}
	return paths
}

func validatePatchPaths(root string, patch string) error {
	for _, path := range sandbox.PatchHeaderPaths(patch) {
		if path == "" || path == "/dev/null" {
			continue
		}
		if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") {
			return fmt.Errorf("patch path %q must stay inside the workspace", path)
		}
		if _, _, err := resolveWorkspaceTargetPath(root, path); err != nil {
			return err
		}
	}
	return nil
}

func recheckPatchWriteTargets(root string, patch string) error {
	for _, path := range sandbox.PatchHeaderPaths(patch) {
		if path == "" || path == "/dev/null" {
			continue
		}
		if err := recheckWorkspaceWriteTarget(root, path); err != nil {
			return err
		}
	}
	return nil
}
