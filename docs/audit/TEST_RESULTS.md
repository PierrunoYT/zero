# Audit Test Results

- **Revision:** `1b5db17`
- **Run date:** 2026-09-08
- **Environment:** Linux x86-64 orb; official Go `1.26.6` installed under `/tmp`
  for the audit because Go was not initially present. Repository files and
  module requirements were not changed to install the toolchain.

## Important environment qualification

The orb's `/etc/gitconfig` sets `commit.gpgsign=true`, but no signing key is
available. Tests that create commits therefore fail for an environment-only
reason unless signing is disabled for their child Git processes. Full plain and
race runs used:

```text
GIT_CONFIG_COUNT=1
GIT_CONFIG_KEY_0=commit.gpgsign
GIT_CONFIG_VALUE_0=false
```

This changes only Git process configuration for the command; no repository or
user config was edited. It avoids misclassifying unavailable orb signing as a
Zero product failure.

## Results

| Command | Outcome | Evidence/notes |
|---|---|---|
| `go version` | PASS | `go version go1.26.6 linux/amd64`; matches `go.mod`. |
| `go env` | PASS | Exit 0; `GOOS=linux`, `GOARCH=amd64`, `GOVERSION=go1.26.6`, and `GOMOD` resolved to the repository `go.mod`. |
| `go mod verify` | PASS | `all modules verified`. |
| `go mod tidy -diff` | PASS | Exit 0, no diff. |
| `gofmt -l .` | PASS | Exit 0, no listed files. |
| `make fmt-check` | PASS | No unformatted Go files. |
| `go vet ./...` | PASS | Exit 0, no diagnostics. |
| `go build ./...` | PASS | Supplemental package compilation check; not a substitute for the repository release build command below. |
| `GIT_CONFIG_COUNT=1 ... go test ./... -count=1` | PASS | All 91 packages; elapsed 3m35.35s, max RSS 720,768 KiB. `anthropic` reported 120.189s and `openai` 180.496s. |
| `GIT_CONFIG_COUNT=1 ... go test -race ./... -count=1` | PASS | All 91 packages; no race report; elapsed 4m11.57s, max RSS 656,724 KiB. `anthropic` reported 121.243s and `openai` 181.584s. |
| `go run ./cmd/zero-release build` | PASS | Built `zero` for linux/amd64, version 0.8.0. |
| `go run ./cmd/zero-release smoke` | PASS | `zero smoke check passed (0.8.0)`. |
| `make lint-static` | PASS | Version-pinned golangci-lint reported `0 issues.` |
| `make deadcode` | PASS with findings | Exit 0; 76 unreachable functions reported: TUI 28, tools 16, sandbox 16, elsewhere 16. Advisory debt, not test failure. |
| `make vulncheck` | PASS | Version-pinned govulncheck: `No vulnerabilities found.` |
| `node --test scripts/action-summary.test.mjs` | PASS | 12 tests passed, 0 failed. |
| `npm audit --package-lock-only --omit=dev` | FAIL (dependency findings) | Exit 1; 2 moderate package results through `tuistory`; details below. No lockfile change. |
| Temporary daemon/ACP short-writer reproductions | PASS (finding reproduced) | Both focused package tests confirmed nil return after incomplete protocol writes; combined run 0.003s. Filed as #1026/#1027; tests removed, not committed. |
| Temporary OAuth slow-header reproduction | PASS (finding reproduced) | `Close` exhausted one second and returned while the accepted connection remained open; focused run 1.20s. Filed as #1028; test removed, not committed. |
| `tuistory@0.10.0` advisory reachability review | PASS (not reachable) | The helper does not import/call the five affected Hono APIs; DEP-01 remains vulnerable inventory, not a demonstrated exploit. |
| Temporary lockfile-only `npm audit fix` | PASS | In a temporary copy, resolved `@hono/node-server@1.19.17` and `hono@4.13.7`; follow-up audit found 0 vulnerabilities. Temporary files were deleted; repository lockfile unchanged. |
| Upstream GitHub tracker reconciliation | PASS | Live API check: 66 open issues, 71 open PRs, and 12 discussions. Issues #1026-#1031 are open/unlabeled; discussions #1002 and #1032-#1036 are open; all five private reports remain in `triage` (two High, three Medium). |
| `git diff --check` | PASS | Exit 0, no whitespace diagnostics. |
| `git diff HEAD --check` | PASS | Exit 0, no whitespace diagnostics. |

## npm audit detail

`npm audit --package-lock-only --omit=dev` exited 1 and reported two moderate
vulnerable package results with fixes available:

| Resolved package | Through | Advisory set |
|---|---|---|
| `@hono/node-server@1.19.14` | `tuistory@0.10.0` | GHSA-frvp-7c67-39w9 |
| `hono@4.12.27` | `@hono/node-server`, `tuistory` | GHSA-8j4g-w8fx-2239; GHSA-f23p-vx2j-j53r; GHSA-79qm-7rj5-m7r9; GHSA-54fx-42gc-7vw4 |

Follow-up source review found that `tuistory@0.10.0` does not invoke any of the
five affected Hono APIs. A temporary lockfile-only audit fix cleared the audit
without changing the repository. This remains dependency inventory, not proof
of a reachable Zero exploit. See DEP-01 in
[Known Issues](KNOWN_ISSUES.md#dep-01--transitive-npm-advisory-exposure).

## Deadcode detail

The version-pinned `make deadcode` target completed successfully and printed 76
unreachable declarations. Distribution by first-party area:

| Area | Count |
|---|---:|
| `internal/tui` | 28 |
| `internal/tools` | 16 |
| `internal/sandbox` | 16 |
| all other packages | 16 |

`deadcode` is whole-program reachability for the current build/platform and can
flag platform adapters, compatibility seams, or intentionally dormant code.
QUAL-01 recommends classification rather than bulk deletion.

## Test coverage and limitations

- Dynamic checks ran only on Linux x86-64. The repository's native CI describes
  macOS and Windows coverage, but those jobs were not rerun in this orb.
- Real provider credentials, real MCP endpoints, OS keyrings, and opt-in native
  sandbox integration were not exercised.
- A passing Go race run detects instrumented memory races in exercised paths; it
  cannot detect filesystem check/use races, cross-process lock mistakes in
  unexercised paths, or platform code skipped on Linux.
- `npm audit` uses current registry advisory metadata and may change after the
  audit date.
- Three temporary regression-style tests were run to verify COR-01 and CON-02,
  then removed; no test or production remediation was committed. Other static
  findings state prerequisites and confidence in their specialist documents.

## Final repository hygiene

The build command can create `./zero`; `.gitignore` intentionally ignores
`/zero`. Generated audit-run binaries are removed before final status. The final
link/citation validation, exact document count, diff checks, and post-commit Git
status are recorded here:

| Check | Final outcome |
|---|---|
| Exactly ten files under `docs/audit` | PASS; exact requested filename set, no extra audit files. |
| Relative Markdown link targets exist | PASS; 365 inline links parsed and every relative target exists. |
| Source/document anchors are valid | PASS; source line ranges are within file length and document heading anchors resolve. |
| `git diff --check` / `git diff HEAD --check` | PASS; no output. |
| Generated `./zero` absent | PASS after removing the ignored 30,752,930-byte build artifact. |
| Final `git status --short` | PASS; no output after the documentation-only commit. |

Interpretation and recommended gates: [Testing Audit](TESTING_AUDIT.md).
