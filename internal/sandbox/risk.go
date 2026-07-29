package sandbox

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	// destructiveCommandPattern matches the highest-risk shell forms:
	//   - rm -rf (with combined/reordered r/f flags) targeting /, $HOME (bare,
	//     quoted, or ${HOME} braced), ~, or *, with an optional `--` before the
	//     target. Each target alternative tolerates optional surrounding quotes
	//     so `rm -rf "/"` / `rm -rf '/'` cannot slip past the gate.
	//   - chmod with combined/reordered flags and an octal-or-777 mode applied
	//     RECURSIVELY (a -R/-r flag) or to root / a sensitive SYSTEM tree
	//     (/, /etc, /usr, /bin, /var, … — e.g. chmod -Rf 777 /, chmod -R 0777 /,
	//     chmod 777 -R /etc, chmod 777 /etc). A single-file chmod 777 — including
	//     an absolute non-system path like `chmod 777 /tmp/build.sh` or a relative
	//     `chmod 777 script.sh` — is intentionally NOT flagged; the intent is
	//     recursive/directory-tree or system-tree chmod.
	//   - mkfs, dd if=, chown -R.
	destructiveCommandPattern = regexp.MustCompile(`(?i)(\brm\s+(-[A-Za-z]*r[A-Za-z]*f|-rf|-fr)\s+(--\s+)?["']?(\$\{?HOME\}?|/|~|\*)["']?|\bmkfs\b|\bdd\s+if=|\bchmod\s+(-[A-Za-z]*[rR][A-Za-z]*\s+)+0?777\b|\bchmod\s+(-\S+\s+)*0?777\s+-[A-Za-z]*[rR][A-Za-z]*\b|\bchmod\s+(-\S+\s+)*0?777\s+["']?/(\s|$|["']|(etc|usr|bin|sbin|lib|lib64|var|boot|opt|root|sys|proc|dev)\b)|\bchown\s+-R\b)`)
	// pipedInstallerPattern matches the fetch-and-execute idiom: a remote fetch
	// (curl/wget/fetch/aria2c) piped into a POSIX shell, with or without a space
	// and across sh/bash/zsh/ksh/dash (so `curl x|sh`, `wget url | bash`, `| zsh`).
	// A purely local pipe into a shell (e.g. `printf … | sh`, `cat ./s | bash`)
	// is NOT a piped installer and must not be flagged.
	pipedInstallerPattern = regexp.MustCompile(`(?i)\b(curl|wget|fetch|aria2c)\b[^|]*\|\s*(ba|z|k|da)?sh\b`)
	// unparseableNetworkPattern is used only after the shell parser fails. At
	// that point the command is already marked too complex, so this intentionally
	// favors catching obvious network programs over proving exact shell syntax.
	// Git needs token-aware handling below: a regex cannot reliably distinguish
	// option values and executable path components from subcommands.
	unparseableNetworkPattern = regexp.MustCompile(`(?i)^(?:(curl|wget|fetch|aria2c|ssh|scp|sftp|rsync|nc|ncat|netcat|telnet|ftp|npx|http-server|vite|next|nuxt|astro)(\s|$)|(npm|pnpm|yarn|bun|pip|pip2|pip3)\s+(install|add|publish|login|start|serve|dev|preview|run\s+(start|serve|dev|preview)|exec|x|dlx)\b|go\s+get\b|python(2|3)?\s+-m\s+(http\.server|pip\s+install)\b|gh\s+(api|repo\s+clone|release\s+download)\b)`)
	// destructiveExtraPatterns hold high-severity patterns that the legacy
	// destructiveCommandPattern does not already cover. Folded in from the
	// blueprint safe_bash.go without duplicating existing matches.
	destructiveExtraPatterns = []*regexp.Regexp{
		// Fork bomb (and minor spacing variants).
		regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
		// Writing to a raw block device (dd of=, redirect to /dev/sdX, etc.).
		regexp.MustCompile(`(?i)>\s*/dev/(sd[a-z]+\d*|nvme\d+n\d+(p\d+)?|hd[a-z]+\d*|xvd[a-z]+\d*|mmcblk\d+)`),
		regexp.MustCompile(`(?i)\bof=/dev/(sd[a-z]+\d*|nvme\d+n\d+(p\d+)?|hd[a-z]+\d*|xvd[a-z]+\d*|mmcblk\d+)`),
		// rm targeting a dangerous root (/, /*, ~, $HOME, *) with ANY mix of
		// short/long flags (incl. --no-preserve-root) in any order, an optional
		// `--` separator, and optional surrounding quotes — so e.g.
		// `rm --no-preserve-root -rf -- "/"` and `rm --no-preserve-root -rf "/"`
		// cannot slip past the gate.
		regexp.MustCompile(`(?i)\brm\s+(-{1,2}\S+\s+)*(--\s+)?["']?(/\*?|~|\$\{?HOME\}?|\*)["']?(\s|$)`),
		// mkfs.<fstype> form (e.g. mkfs.ext4) not caught by the bare \bmkfs\b above when followed by a dot.
		regexp.MustCompile(`(?i)\bmkfs\.[a-z0-9]+\b`),
	}
)

func matchesDestructive(command string) bool {
	if destructiveCommandPattern.MatchString(command) {
		return true
	}
	for _, pattern := range destructiveExtraPatterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

// maxUnparseableShellDepth bounds `sh -c <payload>` recursion in the fallback.
// It IS maxAnalyzerDepth rather than a copy of its value: a fallback that gave
// up a level earlier than the parseable path would drop the network category on
// exactly the deeply-nested launcher chains this path exists to fail closed on.
const maxUnparseableShellDepth = maxAnalyzerDepth

func matchesUnparseableNetwork(command string) bool {
	return matchesUnparseableNetworkAt(command, 0)
}

// matchesUnparseableNetworkAt scans each fallback segment for a network program.
// It resolves the segment's real program the same way the parseable path does —
// past environment assignments and wrapper prefixes (sudo, env, timeout, nice,
// xargs, ...) and the option values those wrappers consume — because a fallback
// that only looked at the first token would let `sudo curl …`, `env git fetch …`,
// or `PATH=.:$PATH git push …` through. The point of this path is to fail closed
// on a command too obfuscated to parse; a wrapper prefix is the cheapest possible
// obfuscation.
//
// Resolving the program (rather than matching the network name anywhere in the
// string, as an earlier revision did) is what keeps `git status push` and
// `echo https://example.com/repo.git push` out: a network verb only counts when
// it belongs to a program actually being invoked.
func matchesUnparseableNetworkAt(command string, depth int) bool {
	for _, tokens := range fallbackCommandTokens(command) {
		body := commandBodyFields(tokens)
		if len(body) == 0 {
			continue
		}
		program, args := executableTokenBase(body[0]), body[1:]
		if program == "git" && matchesUnparseableGitNetwork(args) {
			return true
		}
		if unparseableNetworkPattern.MatchString(strings.Join(append([]string{program}, args...), " ")) {
			return true
		}
		// `sh -c <payload>` runs the payload as a fresh command. The fallback
		// tokenizer keeps a quoted payload as ONE token, so the network program
		// inside it is not a token of this segment at all — recurse the way
		// analyzeInto does on the parseable path.
		if depth < maxUnparseableShellDepth && shellPrograms[program] {
			if payload := fallbackDashCPayload(args); payload != "" {
				if matchesUnparseableNetworkAt(payload, depth+1) {
					return true
				}
			}
		}
	}
	return false
}

// matchesUnparseableGitNetwork reports whether git's arguments (everything after
// the executable) name a subcommand that talks to a remote.
func matchesUnparseableGitNetwork(args []string) bool {
	for index := 0; index < len(args); index++ {
		word := strings.ToLower(args[index])
		if word == "--help" || word == "--version" {
			break
		}
		if strings.HasPrefix(word, "-") {
			if gitGlobalOptionConsumesValue(word) {
				index++
			}
			continue
		}
		switch word {
		case "clone", "fetch", "pull", "push", "ls-remote":
			return true
		case "archive":
			// Only a --remote archive leaves the machine; `git archive HEAD` reads
			// the local object store. See gitUsesNetwork for the same gate on the
			// parseable path.
			return gitTargetsRemoteArchive(args)
		}
		break
	}
	return false
}

// fallbackDashCPayload returns the argument following `-c` or `--command` (the
// command a shell launcher will run), or "" when there is none. Both spellings
// are accepted because bash and zsh accept both, and shellDashCPayload in
// safe_command.go already does — a fallback that only knew `-c` would let
// `bash --command "git push …"` past the very check this path exists for.
func fallbackDashCPayload(args []string) string {
	for index := 0; index < len(args); index++ {
		if isShellCommandFlag(args[index]) && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

// isShellCommandFlag reports whether a token is the flag whose value a shell
// launcher executes.
func isShellCommandFlag(token string) bool {
	return token == "-c" || token == "--command"
}

// executableTokenBase reduces a raw fallback token to a comparable program name.
// It strips quoting, any directory prefix, and a Windows executable suffix, so
// this path recognizes curl.exe and git.cmd exactly as normalizeProgramToken does
// on the parseable path — a token that normalized differently here used to be how
// `curl.exe https://… && "unterminated` lost its network classification.
//
// Drive-relative spellings go through windowsExecutablePathBasename for the same
// reason: `C:git.exe` has no separator to cut on, so a plain basename scan leaves
// `c:git` and the deny never matches a program the parseable path classifies.
func executableTokenBase(token string) string {
	token = strings.Trim(token, `\"'`)
	if basename, ok := windowsExecutablePathBasename(token); ok {
		token = basename
	} else if slash := strings.LastIndexAny(token, `/\`); slash >= 0 {
		token = token[slash+1:]
	}
	return trimExecutableSuffix(strings.ToLower(token))
}

// fallbackCommandTokens performs deliberately small shell/cmd tokenization.
// It preserves quoted spaces even when the command's trailing quote is
// unmatched (the condition that sends classification down this fallback).
func fallbackCommandTokens(command string) [][]string {
	// Command strings commonly preserve cmd.exe's escaped quote spelling.
	command = strings.ReplaceAll(command, `\"`, `"`)
	var commands [][]string
	var tokens []string
	var word strings.Builder
	var quote rune
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	flushCommand := func() {
		flush()
		if len(tokens) > 0 {
			commands = append(commands, tokens)
			tokens = nil
		}
	}
	for _, r := range command {
		if r == '\'' || r == '"' {
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		// A newline separates commands exactly as ;/&/| do. Treating it as mere
		// whitespace kept a multi-line script as one segment, so anything after the
		// first line was scanned as arguments of the first line's program and the
		// program on line two was never resolved.
		if quote == 0 && (r == ';' || r == '&' || r == '|' || r == '\n' || r == '\r') {
			flushCommand()
			continue
		}
		if quote == 0 && (r == ' ' || r == '\t') {
			flush()
			continue
		}
		word.WriteRune(r)
	}
	flushCommand()
	return commands
}

func Classify(request Request) Risk {
	return classifyWithScope(request, nil)
}

func classifyWithScope(request Request, scope *Scope) Risk {
	categories := map[string]bool{}
	level := RiskLow
	add := func(category string, risk RiskLevel) {
		categories[category] = true
		if riskRank(risk) > riskRank(level) {
			level = risk
		}
	}

	switch NormalizeSideEffect(request.SideEffect) {
	case SideEffectRead:
		add("read", RiskLow)
	case SideEffectWrite:
		add("write", RiskMedium)
	case SideEffectShell:
		add("shell", RiskHigh)
	case SideEffectNetwork:
		add("network", RiskHigh)
	case SideEffectLocalControl:
		add("local_control", RiskHigh)
	case SideEffectLocalBrowser:
		add("local_browser", RiskHigh)
	case SideEffectLocalDesktop:
		add("local_desktop", RiskHigh)
	case SideEffectLocalTerminal:
		add("local_terminal", RiskHigh)
	case SideEffectOutOfWorkspace:
		add("out_of_workspace", RiskCritical)
	case SideEffectNone:
		// Control-only tool (e.g. escalate_model): no read/write/shell/network
		// effect, so it contributes no side-effect risk category and stays low.
	}

	// The bash tool accepts the command under any of these aliases; resolve the
	// first non-empty so destructive/network/piped-installer classification
	// cannot be bypassed by choosing a different alias key.
	command := firstArgString(request.Args, "command", "cmd", "script", "shell")
	if command != "" {
		if matchesDestructive(command) {
			add("destructive", RiskCritical)
		}
		if pipedInstallerPattern.MatchString(command) {
			add("piped_installer", RiskCritical)
		}
		// AST second opinion (analyzer.go): walks the parsed shell tree, so it
		// catches destructive/network programs the regexes miss — e.g. shred,
		// fdisk, parted, and commands hidden behind sudo/env wrappers or a
		// `sh -c <payload>` launcher — and flags an unparseable (obfuscated)
		// script as elevated risk. It only ADDS categories, so a benign,
		// parseable command is classified exactly as before.
		analysis := AnalyzeCommand(command)
		if analysis.Network {
			add("network", RiskCritical)
		}
		if analysis.TooComplex && matchesUnparseableNetwork(command) {
			add("network", RiskCritical)
		}
		if analysis.Destructive {
			add("destructive", RiskCritical)
		}
		if analysis.TooComplex {
			add("unparseable_command", RiskHigh)
		}
	}

	for _, path := range requestPaths(request) {
		if filepath.IsAbs(path) {
			add("absolute_path", RiskMedium)
		}
		if path == ".." || strings.HasPrefix(filepath.ToSlash(filepath.Clean(path)), "../") {
			add("path_escape", RiskCritical)
		}
		if request.WorkspaceRoot != "" {
			var block *pathBlock
			if scope != nil {
				block = scope.validate(path)
			} else {
				block = validateWorkspacePath(request.WorkspaceRoot, path)
			}
			if block != nil {
				switch block.Code {
				case BlockSymlinkTraversal:
					add("symlink_traversal", RiskCritical)
				default:
					add("out_of_workspace", RiskCritical)
				}
			}
		}
	}

	names := make([]string, 0, len(categories))
	for category := range categories {
		names = append(names, category)
	}
	sort.Strings(names)
	return Risk{
		Level:      level,
		Categories: names,
		Reason:     riskReason(level, names),
	}
}

func HasRiskCategory(risk Risk, category string) bool {
	for _, candidate := range risk.Categories {
		if candidate == category {
			return true
		}
	}
	return false
}

func riskRank(level RiskLevel) int {
	switch level {
	case RiskLow:
		return 0
	case RiskMedium:
		return 1
	case RiskHigh:
		return 2
	case RiskCritical:
		return 3
	default:
		return 0
	}
}

func riskReason(level RiskLevel, categories []string) string {
	if len(categories) == 0 {
		return string(level)
	}
	return fmt.Sprintf("%s risk: %s", level, strings.Join(categories, ", "))
}

func argString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

// firstArgString returns the first non-empty argument value among keys.
func firstArgString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := argString(args, key); value != "" {
			return value
		}
	}
	return ""
}

func requestPaths(request Request) []string {
	paths := []string{}
	// Keep this aligned with the path-arg alias lists the tools accept (see
	// aliasedStringArg in write_file/edit_file/read_file/grep/glob/list). The
	// sandbox gates by arg-key name, so any alias a tool resolves but the sandbox
	// does not inspect would let a model route a write/read around the
	// workspace+symlink boundary.
	for _, key := range []string{"path", "file", "file_path", "filepath", "filename", "cwd", "workdir", "dir", "directory"} {
		if value := argString(request.Args, key); value != "" {
			paths = append(paths, value)
		}
	}
	if request.ToolName == "apply_patch" {
		paths = append(paths, applyPatchRequestPaths(request.Args)...)
	}
	return paths
}

func applyPatchRequestPaths(args map[string]any) []string {
	patch := firstArgString(args, "patch", "diff")
	if patch == "" {
		return nil
	}
	cwd := firstArgString(args, "cwd")
	var paths []string
	for _, path := range patchHeaderPaths(patch) {
		if path == "" || path == "/dev/null" {
			continue
		}
		if cwd != "" && filepath.Clean(cwd) != "." && !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		paths = append(paths, path)
	}
	return paths
}

func applyPatchPathBlock(request Request) *pathBlock {
	if request.ToolName != "apply_patch" {
		return nil
	}
	patch := firstArgString(request.Args, "patch", "diff")
	if patch == "" {
		return nil
	}
	for _, path := range patchHeaderPaths(patch) {
		if path == "" || path == "/dev/null" {
			continue
		}
		if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") {
			return &pathBlock{
				Code:   BlockOutsideWorkspace,
				Path:   path,
				Reason: fmt.Sprintf("patch path %q must stay inside the workspace", path),
			}
		}
	}
	return nil
}

func patchHeaderPaths(patch string) []string {
	var paths []string
	oldRemaining, newRemaining := 0, 0
	inHunk := false
	for _, line := range strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n") {
		if inHunk && (oldRemaining > 0 || newRemaining > 0) {
			switch {
			case strings.HasPrefix(line, "-"):
				oldRemaining--
			case strings.HasPrefix(line, "+"):
				newRemaining--
			case strings.HasPrefix(line, "\\"):
			default:
				oldRemaining--
				newRemaining--
			}
			continue
		}
		inHunk = false
		switch {
		case strings.HasPrefix(line, "diff --git "):
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				paths = append(paths, stripPatchPrefix(fields[2]), stripPatchPrefix(fields[3]))
			}
		case strings.HasPrefix(line, "@@"):
			oldRemaining, newRemaining = parsePatchHunkCounts(line)
			inHunk = oldRemaining > 0 || newRemaining > 0
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				paths = append(paths, stripPatchPrefix(fields[1]))
			}
		}
	}
	return paths
}

func parsePatchHunkCounts(line string) (int, int) {
	_, rest, ok := strings.Cut(line, "@@")
	if !ok {
		return 0, 0
	}
	rangeSection := rest
	if before, _, ok := strings.Cut(rest, "@@"); ok {
		rangeSection = before
	}
	old, next := 0, 0
	for _, field := range strings.Fields(rangeSection) {
		switch {
		case strings.HasPrefix(field, "-"):
			old = patchHunkCount(field[1:])
		case strings.HasPrefix(field, "+"):
			next = patchHunkCount(field[1:])
		}
	}
	return old, next
}

func patchHunkCount(spec string) int {
	if _, count, ok := strings.Cut(spec, ","); ok {
		if n, err := strconv.Atoi(count); err == nil {
			return n
		}
		return 0
	}
	return 1
}

func stripPatchPrefix(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		path = path[2:]
	}
	return filepath.ToSlash(path)
}
