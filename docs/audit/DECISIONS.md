# Proposed Architecture and Security Decisions

These records capture recommendations from the audit of revision `1b5db17`.
They are **proposed**, not implemented or maintainer-approved. Each should become
a normal ADR/design review before production work.

Upstream decision venues now exist for release authenticity
([discussion #1032](https://github.com/Gitlawb/zero/discussions/1032)), OAuth
storage ([#1033](https://github.com/Gitlawb/zero/discussions/1033)), architecture
([#1002 comment](https://github.com/Gitlawb/zero/discussions/1002#discussioncomment-18351742)),
MCP cancellation contracts ([#1034](https://github.com/Gitlawb/zero/discussions/1034)),
fuzzing ([#1035](https://github.com/Gitlawb/zero/discussions/1035)), and cleanup
error policy ([#1036](https://github.com/Gitlawb/zero/discussions/1036)). A
discussion records a decision request; it is not implementation approval.

## D-01 — Evolve the modular monolith; do not rewrite

- **Status:** Proposed
- **Decision:** retain one Go modular-monolith codebase and current entry
  surfaces, including the deliberately supervised daemon/worker and extension
  process modes. Improve dependency direction through stable facades and small
  application services.
- **Why:** provider, execution, registry, storage, sandbox, and extension
  boundaries are already meaningful and test-rich. The primary architecture
  problem is concentration, not a failed deployment model.
- **Rejected:** microservices or a wholesale package tree rewrite; both enlarge
  compatibility/concurrency surface without addressing the top security risks.
- **Consequence:** progress is measured by ownership, fan-out, and behavior
  parity—not number of new packages.

## D-02 — Filesystem authority is a rooted handle

- **Status:** Proposed
- **Decision:** authorization to read/write/extract under a root is represented
  by an opened rooted capability used for every subsequent operation. A
  canonical string path is not authority.
- **Why:** SEC-02/03/04 all arise because validation and kernel path resolution
  are separate. `internal/pathjail` already uses `os.Root` for this invariant
  ([`pathjail.go`](../../internal/pathjail/pathjail.go#L1-L23)).
- **Rejected:** more `EvalSymlinks`/`Lstat` immediately before pathname open;
  the last check still races. Final-component-only no-follow is also
  insufficient.
- **Consequence:** one cycle-free rooted-I/O package must encode Unix symlink and
  Windows reparse semantics for all components; static validation remains useful
  for UX but is not the security boundary.

## D-03 — Provider credentials do not cross origins implicitly

- **Status:** Proposed
- **Decision:** provider redirects are same-origin by default and reject
  HTTPS-to-HTTP downgrade. Any approved cross-origin flow reconstructs headers
  from a documented non-sensitive allowlist.
- **Why:** arbitrary custom headers and custom auth-header names make generic
  HTTP-client sensitive-header heuristics incomplete.
- **Rejected:** denylisting names such as `X-Api-Key`; unknown custom names and
  future credential schemes remain.
- **Consequence:** define origin as scheme + canonical host + effective port;
  preserve stricter caller redirect policies.

## D-04 — Update input has explicit independent budgets

- **Status:** Proposed
- **Decision:** cap metadata bytes, compressed download bytes, archive entries,
  per-entry expanded bytes, and total expanded bytes independently.
- **Why:** HTTP timeout and checksum verification do not bound fast transfer or
  decompression expansion.
- **Rejected:** trust `Content-Length` alone; chunked transfer and false lengths
  bypass it. A single compressed-size limit does not bound expansion.
- **Consequence:** stream counters fail before promotion, identify the exceeded
  limit, and clean only current-run staging. Exact values need release artifact
  measurements and maintainer approval.

## D-05 — Checksums detect corruption; signatures establish publisher identity

- **Status:** Proposed
- **Decision:** retain per-asset SHA-256 files and add independently verifiable
  release signatures/attestations with a pinned expected identity/workflow.
- **Why:** replacing archive and sibling checksum together defeats same-source
  authenticity while preserving a valid digest relationship.
- **Rejected:** treating GitHub release transport or a sibling checksum alone as
  independent publisher authentication.
- **Consequence:** define key/identity rotation, verification-tool bootstrap,
  offline behavior, rollback, and fail-closed error UX before enforcement.

## D-06 — One canonical permission vocabulary

- **Status:** Proposed
- **Decision:** permission modes/actions and ordering live in one cycle-free
  domain contract. Strings and legacy aliases convert at ingress; unknown values
  fail closed.
- **Why:** agent, swarm, specialist, and CLI mirrors can drift in semantics or
  ordering.
- **Rejected:** keeping synchronized constants by comment/test only.
- **Consequence:** persisted config/session compatibility converters remain
  until old values are inventoried and tested.

## D-07 — One canonical internal tool outcome

- **Status:** Proposed
- **Decision:** use the registry-finalized outcome as the internal tool -> agent
  -> presentation contract; adapt legacy result fields only at external or
  persistence boundaries.
- **Why:** overlapping `tools.Result`/`agent.ToolResult` and legacy/canonical
  fields multiply conversion and redaction assumptions.
- **Rejected:** immediate deletion of legacy fields; old sessions/plugins/API
  surfaces may require them.
- **Consequence:** first inventory encodings and consumers, then dual-read/
  canonical-write through a documented compatibility period. Secret scrubbing
  and output budgeting remain centralized and exactly once.

## D-08 — Long-lived work has explicit ownership and cleanup policy

- **Status:** Proposed
- **Decision:** every goroutine/process/client derives from an owner, has a stop
  mechanism and wait point, and classifies cleanup failure as actionable or
  advisory.
- **Why:** current core managers are strong, but context-ignoring pluggable work
  and discarded cleanup errors weaken shutdown guarantees.
- **Rejected:** a universal lifecycle framework; it could obscure simple local
  ownership. Also reject blanket logging of all close errors, which creates
  noise and can expose secrets.
- **Consequence:** use small local helpers/conventions, `errors.Join` where the
  caller can act, and bounded redaction-safe diagnostics otherwise. Full Linux
  race testing becomes required.

## D-09 — OAuth secrets default to encrypted/keyring storage

- **Status:** Proposed
- **Decision:** align OAuth auto/default selection with the credential store:
  keyring where reliable, encrypted file otherwise, plaintext only by explicit
  choice.
- **Why:** current mode-0600 atomic storage protects against other accounts and
  partial writes but not same-account compromise, backup, or snapshot exposure.
- **Rejected:** silently replacing or deleting existing plaintext stores;
  availability and token loss would outweigh the improvement.
- **Consequence:** transactional, reversible migration with headless/keyring
  recovery guidance and no secret-bearing logs.

## D-10 — Decompose around state ownership, not file size

- **Status:** Proposed
- **Decision:** keep `agent.Run` and the TUI root as stable facades. Extract
  agent state-machine collaborators and TUI feature-owned models one at a time;
  introduce command-specific application services at the CLI edge.
- **Why:** file size and fan-out signal change concentration, but arbitrary
  function moves can make ownership less clear.
- **Rejected:** line-count targets, cosmetic package splitting, or one broad
  `appDeps` service-locator replacement.
- **Consequence:** each extraction needs event/render/session compatibility,
  lifecycle ownership, dependency-direction, and performance evidence.

## D-11 — Test claims at their exact boundary

- **Status:** Proposed
- **Decision:** every security behavior change starts with a deterministic test
  proven to fail without the fix for the stated reason. Native path semantics and
  race checks are mandatory where applicable.
- **Why:** a public-path test can pass because an earlier guard rejects input,
  never reaching the behavior it names. Probabilistic race loops are weak proof.
- **Rejected:** relying on coverage percentage or full-suite green status as
  evidence of a specific boundary claim.
- **Consequence:** add direct seams/fakes sparingly, quote pre-fix failure in the
  PR, and retain the regression after implementation.

## D-12 — Compatibility is an explicit migration gate

- **Status:** Proposed
- **Decision:** CLI, config precedence, session formats, extension contracts, and
  Linux/macOS/Windows behavior are compatibility boundaries. Changes require an
  inventory, fixtures, migration/fallback, and removal date/criterion.
- **Why:** broad architectural cleanup can otherwise turn transitional debt into
  user data loss or platform regressions.
- **Rejected:** using compile success or Linux-only tests as compatibility proof.
- **Consequence:** aliases and adapters can outlive internal migration; remove
  them only after telemetry/evidence available to maintainers and native checks
  demonstrate safety.

## D-13 — Secrets are not accepted as literal command arguments

- **Status:** Proposed
- **Decision:** bearer tokens and future credentials enter through a restrictive
  file, credential store, protected prompt/FD, or explicitly scoped environment;
  public CLI examples and steady-state flows do not place secret bytes in argv.
- **Why:** argv may be copied to shell history, process inspection, diagnostics,
  wrappers, and audit logs. Remote daemon clients currently accept `--token`.
- **Rejected:** relying on redaction after parsing; history/process exposure
  occurs before Zero can redact it.
- **Consequence:** introduce/document token-file input, deprecate the literal
  flag compatibly, and preserve TLS, constant-time comparison, and token-free
  session-link storage.

## D-14 — Protocol writers complete a record or fail

- **Status:** Proposed
- **Decision:** every framed/newline protocol write either emits the complete
  record or returns an error; a nil-error short write becomes
  `io.ErrShortWrite` or is retried through a small `writeAll` helper.
- **Why:** generic `io.Writer` permits short progress, while a truncated daemon
  or ACP record desynchronizes the peer.
- **Rejected:** documenting that current production writers “usually” return an
  error. The exposed generic contract should be correct and directly testable.
- **Consequence:** add partial/zero-progress writer tests without changing wire
  schema, message ordering, or public protocol versions.

Execution order: [Migration Plan](MIGRATION_PLAN.md). Current and target system
maps: [Current Architecture](CURRENT_ARCHITECTURE.md) and
[Target Architecture](TARGET_ARCHITECTURE.md).
