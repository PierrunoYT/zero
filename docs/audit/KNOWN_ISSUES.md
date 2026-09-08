# Known Issues

This is the canonical finding registry for the audit of revision `1b5db17`.
All entries are **open audit findings** unless explicitly marked otherwise; no
remediation was implemented. Severity expresses potential impact under the
listed preconditions, not proof of exploitation.

## Upstream GitHub tracking status

As of 2026-09-08, upstream `Gitlawb/zero` has 63 open issues and 71 open pull
requests. Review of their titles and bodies found:

- private vulnerability reports for SEC-01
  ([GHSA-f484-43mf-99v6](https://github.com/Gitlawb/zero/security/advisories/GHSA-f484-43mf-99v6))
  and SEC-02
  ([GHSA-37cg-763q-376p](https://github.com/Gitlawb/zero/security/advisories/GHSA-37cg-763q-376p)),
  both in `triage` state;
- exact active tracking for TEST-01 ([#939](https://github.com/Gitlawb/zero/issues/939),
  [PR #940](https://github.com/Gitlawb/zero/pull/940)) and PERF-01
  ([PR #955](https://github.com/Gitlawb/zero/pull/955)), plus newly filed exact
  reports for COR-01 ([#1026](https://github.com/Gitlawb/zero/issues/1026),
  [#1027](https://github.com/Gitlawb/zero/issues/1027)) and CON-02
  ([#1028](https://github.com/Gitlawb/zero/issues/1028));
- partial tracking for SEC-04 ([#920](https://github.com/Gitlawb/zero/issues/920),
  [PR #943](https://github.com/Gitlawb/zero/pull/943)) and QUAL-01
  ([#904](https://github.com/Gitlawb/zero/issues/904),
  [PR #975](https://github.com/Gitlawb/zero/pull/975)); and
- no active tracking for the other 16 findings.

Related work on atomic tool writes, keyring capacity, daemon token-file
protection, and workflow permissions does not cover SEC-02, SEC-07, SEC-08, or
SEC-09 respectively. Audit IDs remain local identifiers, not GitHub issue
numbers. The advisory links require participant access until coordinated
disclosure. Detailed overlap and residual-scope analysis is in
[Codebase Audit](CODEBASE_AUDIT.md#open-upstream-issue-and-pull-request-reconciliation).
Issues #1026-#1028 are open and currently unlabeled; an attempted `bug` label
update returned HTTP 403 because only upstream maintainers can perform that
triage.

## Ranking method

Rank combines impact, plausible exposure, confidence in the observed mechanism,
and remediation urgency. Security findings use High/Medium/Low; engineering
findings use the same scale for prioritization. “Medium-high” separates items
that should follow the high-priority boundary fixes from ordinary medium debt.

## Top 10

| Rank | ID | Severity | Owner area | Finding | Status |
|---:|---|---|---|---|---|
| 1 | SEC-01 | High | Providers | Cross-origin redirects may retain custom authentication headers. | Private GHSA-f484-43mf-99v6; triage |
| 2 | SEC-02 | High | Tools/filesystem | Workspace write/edit checks are separated from pathname writes. | Private GHSA-37cg-763q-376p; triage; #921/PR #941 address atomicity only |
| 3 | SEC-03 | Medium-high | MCP/filesystem | Resource scope is decided before a separate pathname read. | Open; regression needed |
| 4 | SEC-05 | Medium-high | Updater | Downloads and expanded archives lack byte/entry limits. | Open; limits undecided |
| 5 | SEC-08 | Medium | Remote daemon | Client bearer tokens are accepted in process arguments. | Open; compatibility deprecation needed |
| 6 | SEC-04 | Medium | Updater | Archive extraction confinement is pathname-based. | Partial #920/PR #943; rooted-race residual open |
| 7 | SEC-06 | Medium | Release | Sibling SHA-256 assets do not independently authenticate artifacts. | Open; signing design needed |
| 8 | SEC-07 | Medium | OAuth | File token storage defaults to mode-0600 plaintext. | Open; #937/PR #1007 solve a different keyring limit |
| 9 | TEST-01 | Medium | CI | Full race detection is not a required CI gate. | Exact #939; PR #940 open |
| 10 | DEP-01 | Medium | Node helper | npm reports two moderate vulnerable transitive packages. | Open; applicability unproven |

## Detailed registry

### SEC-01 — Cross-origin custom-auth redirect exposure

- **Evidence:** provider requests apply custom/configured headers before HTTP
  execution, while retry I/O follows redirects without a Zero-defined origin
  classification
  ([`retry.go`](../../internal/providers/providerio/retry.go#L98-L146),
  [`headers.go`](../../internal/providers/providerio/headers.go#L8-L52)).
- **Preconditions:** a credential is carried in a custom/non-standard header and
  the configured endpoint returns a redirect to another origin.
- **Potential impact:** provider credential disclosure to the redirect target.
- **Qualification:** Go already protects recognized sensitive headers in common
  cross-host redirects; no production endpoint exploit was demonstrated.
- **Remediation:** same-origin-only default; reject downgrade; rebuild explicitly
  permitted cross-origin requests from a non-sensitive allowlist.
- **Acceptance:** tests prove same-origin behavior and prove that cross-origin
  `Authorization`, cookies, custom auth names, and credential-like custom headers
  cannot arrive unintentionally; existing caller policies remain stricter.
- **Detail:** [Security Audit, SEC-01](SECURITY_AUDIT.md#sec-01--cross-origin-redirects-may-retain-custom-authentication-headers).

### SEC-02 — Workspace write pathname TOCTOU

- **Evidence:** component `Lstat` recheck precedes absolute-path `os.WriteFile`
  ([`workspace.go`](../../internal/tools/workspace.go#L140-L198),
  [`write_file.go`](../../internal/tools/write_file.go#L58-L110)); edit uses the
  same pattern ([`edit_file.go`](../../internal/tools/edit_file.go#L149-L157)).
- **Preconditions:** a concurrent process can mutate a traversed in-scope
  directory between check and open.
- **Potential impact:** write outside the authorized root, limited further by
  whatever OS sandbox policy is actually enforced.
- **Qualification:** static traversal and observed symlinks are rejected; this
  is a race window, not an ordinary path-string bypass claim.
- **Remediation:** rooted, handle-relative parent creation and atomic write/
  replace on every supported platform.
- **Acceptance:** deterministic component-swap tests fail on old pathname code
  and pass on rooted code for create, overwrite, and edit; native Windows tests
  cover reparse points; no stale-content behavior regresses.
- **Detail:** [Security Audit, SEC-02](SECURITY_AUDIT.md#sec-02--workspace-write-tools-have-a-check-to-use-pathname-window).

### SEC-04 — Update extraction pathname TOCTOU

- **Severity:** Medium.

- **Evidence:** names are checked lexically, then directories, links, and files
  are created by pathname
  ([`extract.go`](../../internal/update/extract.go#L38-L85),
  [`extract.go`](../../internal/update/extract.go#L139-L170)).
- **Preconditions:** a same-user actor can mutate descendants in the private
  update staging tree during extraction.
- **Potential impact:** archive content written outside the intended extraction
  root under a successful concurrent swap.
- **Qualification:** the actor normally already has the same account's
  filesystem authority; checksum verification and a private temporary parent
  further lower exposure. No privilege expansion or static single-archive escape
  was demonstrated.
- **Remediation:** extract through one rooted destination authority; reject
  symlinks or guarantee they can never redirect later traversal.
- **Acceptance:** adversarial order/swap/symlink-chain tests, native reparse
  tests, and cleanup/no-promotion tests pass; ordinary zip/tar releases remain
  compatible.
- **Detail:** [Security Audit, SEC-04](SECURITY_AUDIT.md#sec-04--update-extraction-confinement-is-pathname-based).

### SEC-03 — MCP resource canonicalize-then-read TOCTOU

- **Evidence:** `EvalSymlinks` and root comparison return a path that is later
  separately statted/read
  ([`resources.go`](../../internal/mcp/resources.go#L142-L164),
  [`resources.go`](../../internal/mcp/resources.go#L187-L215)).
- **Preconditions:** an MCP resource request and a concurrent local actor able to
  replace a traversed path component.
- **Potential impact:** file content outside the allowed root returned to the
  MCP peer.
- **Qualification:** traversal, static external symlinks, non-regular files, and
  oversized resources already fail.
- **Remediation:** rooted open, then type/size/read from that exact object.
- **Acceptance:** controlled swap tests fail on old code and pass through rooted
  reads; read limits and existing URI compatibility remain.
- **Detail:** [Security Audit, SEC-03](SECURITY_AUDIT.md#sec-03--mcp-resource-scope-decision-is-separated-from-file-open).

### SEC-05 — Unbounded updater input and expansion

- **Evidence:** metadata decode and archive download are not byte-limited;
  extraction has no file/entry/total expansion budget
  ([`update.go`](../../internal/update/update.go#L334-L359),
  [`apply.go`](../../internal/update/apply.go#L362-L390),
  [`extract.go`](../../internal/update/extract.go#L38-L85)).
- **Preconditions:** release source, network path, or metadata can provide a
  maliciously large response/archive; available disk/time matters.
- **Potential impact:** disk exhaustion, excessive I/O/CPU, failed update.
- **Qualification:** HTTP context timeout bounds time, not fast bytes; checksums
  authenticate bytes only after download and do not constrain expansion.
- **Remediation:** documented limits for metadata, download, entries, one file,
  and total expanded bytes; fail and clean up before promotion.
- **Acceptance:** exact-boundary success plus chunked over-limit, compression
  ratio, many-entry, truncated, and cleanup tests; errors identify the limit.
- **Detail:** [Security Audit, SEC-05](SECURITY_AUDIT.md#sec-05--go-updater-does-not-bound-downloaded-or-expanded-input).

### SEC-06 — Same-source release checksum authenticity

- **Evidence:** updater fetches archive and `.sha256` sibling from the same
  release metadata/source
  ([`apply.go`](../../internal/update/apply.go#L164-L174),
  [`apply.go`](../../internal/update/apply.go#L241-L255)).
- **Preconditions:** compromise or unauthorized control of the release channel
  sufficient to replace both assets.
- **Potential impact:** a malicious replacement can have a matching replacement
  checksum.
- **Qualification:** current checksum handling strongly detects corruption,
  missing/wrong assets, and accidental mismatch; it should be retained.
- **Remediation:** independently verifiable artifact signature/attestation with a
  pinned expected identity and workflow.
- **Acceptance:** valid signed releases install; archive or checksum/signature
  substitution fails before extraction; rotation and offline/failure behavior are
  documented and tested.
- **Detail:** [Security Audit, SEC-06](SECURITY_AUDIT.md#sec-06--sibling-checksums-do-not-independently-authenticate-releases).

### SEC-07 — OAuth plaintext file default

- **Evidence:** empty/default storage selects the plaintext file backend
  ([`oauth/store.go`](../../internal/oauth/store.go#L84-L99),
  [`oauth/store.go`](../../internal/oauth/store.go#L161-L189)).
- **Preconditions:** same-account compromise, readable backup/snapshot, or
  accidental disclosure of the mode-0600 file.
- **Potential impact:** bearer/refresh token disclosure at rest.
- **Qualification:** mode-0700 directories, mode-0600 atomic files, and locking
  are strong; another OS user does not gain access from this behavior alone.
- **Remediation:** encrypted/keyring auto default aligned with API-key storage;
  explicit reversible migration and plaintext opt-in.
- **Acceptance:** existing stores migrate without token loss, rollback is
  defined, permissions stay restrictive, headless failures are actionable, and
  logs never include tokens.
- **Detail:** [Security Audit, SEC-07](SECURITY_AUDIT.md#sec-07--oauth-file-storage-defaults-to-plaintext).

### SEC-08 — Remote daemon token accepted in argv

- **Evidence:** `daemon run`, `attach`, and `link` parse literal `--token`
  arguments
  ([`daemon.go`](../../internal/cli/daemon.go#L276-L317),
  [`daemon.go`](../../internal/cli/daemon.go#L371-L410),
  [`daemon.go`](../../internal/cli/daemon.go#L587-L639)).
- **Preconditions:** the operator selects the optional flag and command history
  or process arguments are visible to another local principal/process.
- **Potential impact:** disclosure of a bearer token that authorizes remote
  daemon sessions or bundle upload.
- **Qualification:** TLS is mandatory, token comparison is constant-time,
  environment/token-file alternatives already exist, and session links never
  persist the token.
- **Remediation:** deprecate literal token arguments; document environment/file
  input and optionally add `--token-file`; warn without echoing secrets.
- **Acceptance:** help and examples contain no literal-token recommendation;
  token-free argv is proven; errors/logs/link files contain no token; old flag
  removal follows a documented compatibility window.
- **Detail:** [Security Audit, SEC-08](SECURITY_AUDIT.md#sec-08--remote-daemon-bearer-tokens-are-accepted-in-argv).

### TEST-01 — Race detector absent from CI

- **Evidence:** `make test` includes `-race`, but the CI matrix runs plain tests
  ([`Makefile`](../../Makefile#L20-L26),
  [`ci.yml`](../../.github/workflows/ci.yml#L74-L85)).
- **Impact:** future Go memory races may merge despite a currently passing suite.
- **Remediation:** required full Linux race job; retain native plain matrix.
- **Acceptance:** branch protection requires the race job, it runs all packages
  with `-count=1`, and failures cannot be converted to advisory success.
- **Detail:** [Concurrency Audit](CONCURRENCY_AUDIT.md#test-01--race-detection-is-not-a-ci-gate).

### DEP-01 — Transitive npm advisory exposure

- **Evidence:** npm audit reports moderate results for
  `@hono/node-server@1.19.14` and `hono@4.12.27` via `tuistory@0.10.0`.
- **Potential impact:** depends on whether Zero's helper invokes the affected
  Windows static path, CORS, SSR memo, proxy, or language middleware paths.
- **Qualification:** applicability was not established; this is not a confirmed
  exploitable Zero path.
- **Remediation:** map helper call paths, test the supported Node/platform matrix,
  and upgrade the direct dependency/lockfile when compatible.
- **Acceptance:** audit is clean or every remaining advisory has a versioned,
  evidence-backed reachability decision and review date; helper tests pass.
- **Detail:** [Testing Audit](TESTING_AUDIT.md#dependency-and-supply-chain-testing).

### ARCH-01 — Large CLI/TUI/agent concentration

- **Evidence:** `tui/model.go` is 6,064 lines, `agent/loop.go` 3,487, and
  `cli/app.go` 1,589; CLI imports 59 internal packages and TUI 39.
- **Impact:** high review surface, implicit state coupling, expensive regression
  analysis, and harder ownership/lifecycle reasoning.
- **Qualification:** size/fan-out are architecture indicators, not correctness
  defects; broad tests and stable facades reduce immediate risk.
- **Remediation:** command services, agent collaborators, and feature-owned TUI
  models behind current facades; one behavior-preserving extraction per change.
- **Acceptance:** dependency direction improves without cycles; golden/event/
  session compatibility and performance baselines remain green; no big-bang
  package move.
- **Detail:** [Current Architecture](CURRENT_ARCHITECTURE.md#current-pressure-points).

## Additional findings

| ID | Severity | Finding | Evidence / acceptance summary |
|---|---|---|---|
| ARCH-02 | Medium | Config resolution imports and validates runtime domains. | [`resolver.go`](../../internal/config/resolver.go#L1-L17); separate merge/normalization from narrow validators while preserving exact precedence and trust gates. |
| ARCH-03 | Medium | Permission identifiers and parsing are mirrored across agent/swarm/specialist/CLI. | [`agent/types.go`](../../internal/agent/types.go#L20-L71), [`swarm/team.go`](../../internal/swarm/team.go#L290-L320); one cycle-free enum, aliases only at ingress, unknown fails closed. |
| ARCH-04 | Medium | `tools.Result` and `agent.ToolResult` overlap and carry legacy fields. | [`tools/types.go`](../../internal/tools/types.go#L95-L152), [`agent/types.go`](../../internal/agent/types.go#L73-L128); one canonical internal outcome after persisted/API compatibility inventory. |
| REL-01 | Medium | Stateful cleanup errors are discarded or only partly reported. | Agent/MCP/execution/daemon examples in [Concurrency Audit](CONCURRENCY_AUDIT.md#rel-01--stateful-cleanup-errors-are-inconsistently-observable); classify, join actionable errors, otherwise redact-safe diagnostics. |
| CON-01 | Low-medium | MCP timeout reaper can remain if a client factory ignores context forever. | [`registry.go`](../../internal/mcp/registry.go#L115-L143); require/test context compliance or an independently closable/bounded worker. |
| CON-02 | Low-medium | OAuth loopback HTTP servers have no I/O bounds or joined Serve completion. | Exact [issue #1028](https://github.com/Gitlawb/zero/issues/1028); a temporary slow-header test confirmed an open accepted connection after the one-second close budget. Details: [Concurrency Audit](CONCURRENCY_AUDIT.md#con-02--oauth-loopback-servers-lack-io-bounds-and-a-joined-lifecycle). |
| COR-01 | Low-medium | Daemon and ACP generic writers do not detect nil-error short writes. | Exact [issue #1026](https://github.com/Gitlawb/zero/issues/1026) and [issue #1027](https://github.com/Gitlawb/zero/issues/1027); temporary tests confirmed silent incomplete records. Write completely or return `io.ErrShortWrite`. |
| SEC-09 | Low | Release checkout retains workflow Git credentials by default. | [`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L33-L43), [`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L126-L133); disable persistence unless a later Git step requires it. |
| QUAL-01 | Low-medium | Advisory deadcode reports 76 unreachable functions. | Partial #904/PR #975 concerns a smaller `-test` set; classify the repository target's 76 `-test=false` results by platform/compatibility/dormancy/removability. |
| TEST-02 | Medium | Priority redirect/path/update negative interleavings lack regression tests. | Add direct deterministic tests and prove each fails for the claimed reason before remediation. |
| TEST-03 | Low-medium | No Go fuzz targets were found. | Seed high-risk parsers; preserve deterministic regression for every finding. |
| TEST-04 | Low-medium | Node action-summary tests and npm audit are not main-CI gates. | Add a small shipped-wrapper job or document why that surface is released elsewhere. |
| PERF-01 | Low-medium | Full test feedback is dominated by real-time provider tests. | Exact remediation is open in PR #955; baseline was 3m35s plain and 4m11s race in this orb. Preserve realistic integration coverage while reviewing its test-time backoff/parallelism approach. |

## Not findings / controls to preserve

- The audit did **not** find evidence that project/provider command config can
  silently disable mandatory sandbox policy; trust resolution rejects those
  changes.
- The audit did **not** find an ordinary static archive traversal that bypasses
  the updater's lexical checks. SEC-04 is about concurrent resolution.
- The audit did **not** observe a Go data race in the full Linux race run.
- Mode-0600 plaintext OAuth storage is not world-readable; SEC-07 compares
  at-rest defense depth with the stronger credential-store default.
- Sibling checksums remain valuable corruption and attribution checks despite
  SEC-06.
- Remote daemon TLS, constant-time authentication, bounded handshakes/
  connections/bundles, and token-free mode-0600 link files remain strong despite
  the optional argv input in SEC-08.
- The GitHub Action summary is currently normalized to one line before the fixed
  output delimiter, so delimiter injection was not retained as a finding.
- `tools.Registry` central redaction/output budgeting, rooted `pathjail`, atomic
  store publication, fail-closed trust gates, and bounded process/MCP output are
  sound patterns to extend, not replace.

Remediation sequence: [Migration Plan](MIGRATION_PLAN.md). Proposed policy
choices: [Decisions](DECISIONS.md).
