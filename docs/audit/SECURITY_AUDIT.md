# Security Audit

- **Revision:** `1b5db17`
- **Method:** static control/data-flow review plus repository tests and the
  checks in [Test Results](TEST_RESULTS.md).
- **Interpretation:** “risk” means the code permits a concerning sequence under
  stated preconditions. It does not mean that sequence was demonstrated against
  a real installation.

## Threat model used

The review considered:

- an untrusted repository and processes it starts while Zero is operating;
- a configured or compromised provider/MCP endpoint;
- a remote MCP client with access to Zero's MCP server;
- a remote daemon client or local shell/process able to inspect command lines;
- another process running as the same local user and able to race writable
  workspace or temporary paths;
- a compromised release source/publisher or maliciously large release response;
- local users/processes outside the account that owns mode-0600 files.

It did not assume kernel compromise, arbitrary memory access in the Zero
process, or that mode-0600 files are readable by a different OS account.

## Severity model

- **High:** plausible path to secret disclosure or write outside an authorized
  root with substantial impact; exploitation may still require a stated race or
  configuration.
- **Medium:** meaningful defense-in-depth, availability, local confidentiality,
  or supply-chain weakness with stronger prerequisites.
- **Low:** limited impact, hard prerequisites, or primarily diagnostic debt.

## Findings

### SEC-01 — Cross-origin redirects may retain custom authentication headers

- **Severity:** High
- **Confidence:** high in the mechanism; exploitability configuration-dependent.
- **Private upstream report:**
  [GHSA-f484-43mf-99v6](https://github.com/Gitlawb/zero/security/advisories/GHSA-f484-43mf-99v6)
  (`triage`; advisory participants only until coordinated disclosure).

**Observed behavior.** Provider I/O clones the caller's HTTP client and wraps
`CheckRedirect`, but follows redirects by default
([`retry.go`](../../internal/providers/providerio/retry.go#L98-L121)). Each
attempt builds a request and applies the complete configured header callback
before `Do` ([`retry.go`](../../internal/providers/providerio/retry.go#L133-L146)).
The auth helper supports arbitrary `CustomHeaders` and a custom auth-header name
([`headers.go`](../../internal/providers/providerio/headers.go#L8-L25),
[`headers.go`](../../internal/providers/providerio/headers.go#L27-L52)); provider
profiles expose both
([`config/types.go`](../../internal/config/types.go#L25-L40)). There is no Zero
redirect policy that classifies/removes custom credential-bearing headers when
the host/origin changes.

**Risk.** Go protects known sensitive headers such as `Authorization` in common
cross-host cases, but an API key can be configured in `X-Api-Key` or another
custom name. Such a header may be copied to a redirect target. A provider or
intermediary able to produce a cross-origin redirect could therefore receive a
credential intended for the configured origin.

**Preconditions/limits.** A request must carry auth in a non-standard/custom
header, and a redirect to another origin must occur. The audit did not prove a
production provider currently emits such a redirect or demonstrate credential
exfiltration. Existing redirect tests prove follow/no-replay/caller-policy
semantics but do not assert header behavior
([`retry_test.go`](../../internal/providers/providerio/retry_test.go#L391-L486)).

**Recommendation.** Default provider completion redirects to same-origin only.
For any allowed cross-origin case, rebuild from a strict non-sensitive header
allowlist and never forward configured auth/custom headers implicitly. Compare
scheme, canonical host, and effective port. Preserve the caller's stricter
policy.

**Required regression tests.** Same-origin redirect retains expected auth;
cross-origin redirect rejects or strips `Authorization`, custom auth header,
cookies, and credential-like custom fields; benign explicitly allowlisted
headers behave as designed; HTTPS-to-HTTP downgrade is refused.

### SEC-02 — Workspace write tools have a check-to-use pathname window

- **Severity:** High
- **Confidence:** high in the race window; exploitation requires a concurrent
  filesystem actor.
- **Private upstream report:**
  [GHSA-37cg-763q-376p](https://github.com/Gitlawb/zero/security/advisories/GHSA-37cg-763q-376p)
  (`triage`; advisory participants only until coordinated disclosure).

**Observed behavior.** `write_file` resolves and checks a target, optionally
reads current content, creates parent directories, rechecks each path component,
then calls `os.WriteFile` by absolute pathname
([`write_file.go`](../../internal/tools/write_file.go#L58-L110)). `edit_file`
similarly rechecks then writes by pathname
([`edit_file.go`](../../internal/tools/edit_file.go#L149-L157)). The recheck uses
component-by-component `Lstat`
([`workspace.go`](../../internal/tools/workspace.go#L140-L198)). A component can
change after its `Lstat` and before the final write resolves the path.

**Risk.** A concurrent process that can replace an in-scope path component with
a symlink/reparse point in that interval may redirect a write outside the
authorized scope. The stale-file tracker prevents accidental overwrite based on
changed content, but it is not an open-time containment primitive.

**Preconditions/limits.** The attacker needs concurrent mutation rights to a
traversed directory. Static traversal and existing symlink cases are rejected;
the risk is the swap between check and use. The normal OS sandbox may reduce
what the Zero process itself can write, but an explicitly granted extra root or
degraded/disabled sandbox changes that containment, and tool authorization
should remain correct independently.

**Positive local precedent.** `internal/pathjail` explicitly documents this
class and uses `os.Root` so descendant resolution is handle-relative
([`pathjail.go`](../../internal/pathjail/pathjail.go#L1-L23),
[`pathjail.go`](../../internal/pathjail/pathjail.go#L45-L125)). Its exclusive
rooted temporary creation also avoids predictable-name substitution
([`pathjail.go`](../../internal/pathjail/pathjail.go#L144-L176)).

**Recommendation/tests.** Perform parent creation and atomic create/replace via
a rooted handle, applying Windows reparse-point protections to every component.
Add deterministic swap-at-open seams; prove the tests fail on pathname writes,
then cover create, overwrite, edit, symlink, junction/reparse, and each supported
platform's expected error set.

### SEC-03 — MCP resource scope decision is separated from file open

- **Severity:** Medium-high
- **Confidence:** high in the window; exploitation requires a concurrent local
  filesystem actor and an MCP read request.

**Observed behavior.** Resource reads call `EvalSymlinks`, compare the canonical
path against canonical allowed roots, and return a string path
([`resources.go`](../../internal/mcp/resources.go#L187-L215)). The caller later
stats and reads that path separately
([`resources.go`](../../internal/mcp/resources.go#L142-L164)). A component can
be swapped after the containment decision.

**Risk.** A remote MCP client might receive bytes from a file that no longer
corresponds to the path that passed scope validation.

**Existing controls.** Static traversal, out-of-scope absolute paths, and an
in-root symlink to an external file are tested and rejected
([`resources_test.go`](../../internal/mcp/resources_test.go#L157-L209)). Reads
are capped at the MCP framing limit before allocation
([`resources.go`](../../internal/mcp/resources.go#L26-L29),
[`resources.go`](../../internal/mcp/resources.go#L150-L159)).

**Recommendation/tests.** Open through a rooted read handle, verify type/size on
the opened object, and read that same object with a hard limit. Add a controlled
component swap between validation and open, including Windows reparse semantics.

### SEC-04 — Update extraction confinement is pathname-based

- **Severity:** Medium
- **Confidence:** high in the window; practical exposure is reduced by updater
  staging.

**Observed behavior.** Archive names receive a lexical traversal check
([`extract.go`](../../internal/update/extract.go#L155-L170)); extraction then
uses pathname `MkdirAll`, `Symlink`, and `OpenFile`
([`extract.go`](../../internal/update/extract.go#L47-L79),
[`extract.go`](../../internal/update/extract.go#L139-L152)). Tar archives may
contain symlinks whose target is lexically inside the destination. No rooted
handle binds the later operations to the destination object.

**Risk.** A same-account actor able to discover and swap a staging descendant at
the right time could redirect extraction outside its destination. That actor
normally already has the account's filesystem authority, so this audit did not
demonstrate privilege or sandbox-boundary expansion. This is confinement
hardening against a concurrent pathname race, not a claim that one ordinary
archive bypasses the reviewed static `../` checks.

**Preconditions/limits.** Standalone update creates a private temporary parent
and verifies the archive checksum before extraction
([`apply.go`](../../internal/update/apply.go#L147-L180)); those controls reduce
exposure. Existing tests reject lexical traversal, zip symlinks, and escaping
tar symlink targets, and allow an intentional in-tree tar symlink
([`extract_test.go`](../../internal/update/extract_test.go#L116-L189),
[`extract_test.go`](../../internal/update/extract_test.go#L225-L309)). They do
not drive a concurrent swap.

**Recommendation/tests.** Extract exclusively relative to an opened destination
root. Either reject archive symlinks or create them with a policy that cannot
affect subsequent traversal. Add entry-order, symlink-chain, concurrent-swap,
and Windows reparse tests.

### SEC-05 — Go updater does not bound downloaded or expanded input

- **Severity:** Medium-high
- **Confidence:** high.

`downloadFile` streams an HTTP body to disk with unbounded `io.Copy`
([`apply.go`](../../internal/update/apply.go#L362-L390)); release metadata JSON is
decoded without a body limit
([`update.go`](../../internal/update/update.go#L334-L359)). Archive extraction
has no entry-count, per-entry, or cumulative expanded-byte cap, and file copy is
unbounded ([`extract.go`](../../internal/update/extract.go#L38-L85),
[`extract.go`](../../internal/update/extract.go#L139-L152)). A context timeout
bounds elapsed time but not bytes written quickly or decompression expansion.

The npm fallback has a 512 MiB declared/actual download cap and blocks insecure
redirect downgrades
([`postinstall.mjs`](../../scripts/postinstall.mjs#L44-L50),
[`postinstall.mjs`](../../scripts/postinstall.mjs#L163-L196)), demonstrating a
nearby policy precedent.

**Recommendation.** Define separate, documented limits for metadata, archive
download, entries, one expanded file, and total expansion. Enforce with
`io.LimitedReader`/counting writers and abort before promotion. Test chunked
responses without `Content-Length`, boundary values, high compression ratios,
many empty entries, cleanup after rejection, and no partial install.

### SEC-06 — Sibling checksums do not independently authenticate releases

- **Severity:** Medium
- **Confidence:** high as a trust-model observation.

The updater downloads the archive and `.sha256` sibling from release metadata,
then validates digest and expected filename
([`apply.go`](../../internal/update/apply.go#L164-L174),
[`apply.go`](../../internal/update/apply.go#L241-L255)). Release code creates and
checks those digest files
([`release.go`](../../internal/release/release.go#L412-L466)). This robustly
detects corruption, wrong-file attribution, missing checksums, and mismatches.
It cannot distinguish legitimate assets from replacement of both archive and
checksum by an actor controlling the same release channel. The JS installer
states this limitation explicitly
([`postinstall.mjs`](../../scripts/postinstall.mjs#L44-L48)).

**Recommendation.** Keep checksums for corruption detection and add an
independently verifiable signature/attestation identity for release artifacts.
Pin the expected identity/workflow and verify before extraction. Preserve npm's
strong OIDC/provenance and reviewed environment controls
([`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L108-L145)).

### SEC-07 — OAuth file storage defaults to plaintext

- **Severity:** Medium
- **Confidence:** high.

The unified OAuth token store documents `file` as the default and returns a
plaintext file backend when storage is empty
([`oauth/store.go`](../../internal/oauth/store.go#L84-L99),
[`oauth/store.go`](../../internal/oauth/store.go#L161-L189)). The file is
published atomically via a 0700 directory and random mode-0600 temporary file,
under a cross-process lock
([`oauth/store.go`](../../internal/oauth/store.go#L393-L468)). Optional
AES-GCM and keyring backends exist.

**Risk/limit.** This does not grant another OS account permission to read the
tokens. It does leave bearer/refresh tokens unencrypted for same-account
malware, backups, snapshots, or accidental file disclosure. In contrast, API
keys never silently downgrade to plaintext: they default to keyring on supported
macOS or encrypted file elsewhere
([`credstore.go`](../../internal/credstore/credstore.go#L69-L117)).

**Recommendation.** Align OAuth auto/default behavior with `credstore`, migrate
existing plaintext explicitly and reversibly, retain an opt-in plaintext mode,
and ensure headless/keyring-unavailable failures have actionable recovery.

### SEC-08 — Remote daemon bearer tokens are accepted in argv

- **Severity:** Medium
- **Confidence:** high in exposure; impact depends on host process/history
  visibility.

Remote daemon `run` and `attach` parse both `--token value` and `--token=value`
([`daemon.go`](../../internal/cli/daemon.go#L276-L317),
[`daemon.go`](../../internal/cli/daemon.go#L371-L410)); `link` advertises and
parses the same form before constructing the authenticated client
([`daemon.go`](../../internal/cli/daemon.go#L52-L75),
[`daemon.go`](../../internal/cli/daemon.go#L587-L639)). A literal supplied this
way can remain in shell history and can be visible in process-argument
inspection while the client runs.

The bridge itself correctly requires TLS, compares a nonempty token in constant
time, does not log it, and supports `ZERO_DAEMON_REMOTE_TOKEN` or
`ZERO_DAEMON_REMOTE_TOKEN_FILE`
([`auth.go`](../../internal/daemon/remote/auth.go#L1-L17),
[`auth.go`](../../internal/daemon/remote/auth.go#L33-L86)). Saved session links
explicitly omit tokens
([`sessionlink.go`](../../internal/daemon/remote/sessionlink.go#L10-L30)). The
risk is therefore an optional client-input path, not wire plaintext or link-file
persistence.

**Recommendation/tests.** Deprecate and remove literal `--token` after a
compatibility window; make environment/token-file input the documented path and
consider an explicit `--token-file`. Until removal, warn without echoing the
value. Tests should inspect spawned argv/help/diagnostics, verify token-file
permissions/error redaction, and prove saved links remain secret-free.

### SEC-09 — Release checkouts retain workflow credentials

- **Severity:** Low
- **Confidence:** high as defense-in-depth.

CI explicitly configures `actions/checkout` with `persist-credentials: false`
([`ci.yml`](../../.github/workflows/ci.yml#L20-L26)), but package and npm release
checkouts retain checkout's default Git credential persistence
([`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L33-L43),
[`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L126-L133)).
The package matrix has `contents: write`; later publication already receives
`GH_TOKEN` explicitly. This is avoidable token availability to trusted build
steps, not evidence that a current script exfiltrates it.

**Recommendation/tests.** Set `persist-credentials: false` on every release
checkout unless a specific later Git operation proves it is required. Keep
least-privilege job permissions and explicit step-scoped publication tokens.

## Dependency advisory exposure

`npm audit --package-lock-only --omit=dev` reported two moderate vulnerable
transitive packages through `tuistory@0.10.0`:
`@hono/node-server@1.19.14` (GHSA-frvp-7c67-39w9) and `hono@4.12.27` (four
advisories). Fixes are reported available. Zero vendors these helpers for local
browser/terminal control; the audit did not prove that Zero invokes the affected
Hono static, CORS, memo, proxy, or language paths. Treat DEP-01 as exposed
dependency inventory pending an applicability test, not a confirmed Zero
vulnerability. See [Testing Audit](TESTING_AUDIT.md#dependency-and-supply-chain-testing).

## Controls to preserve

1. **Fail-closed trust layering.** Provider-command config cannot disable the
   sandbox, and untrusted project config cannot start MCP servers
   ([`config/resolver.go`](../../internal/config/resolver.go#L96-L107),
   [`config/resolver.go`](../../internal/config/resolver.go#L182-L215)).
2. **Sandbox policy.** Defaults deny network and enforce workspace boundaries
   ([`sandbox/types.go`](../../internal/sandbox/types.go#L256-L261)); required
   native enforcement and explicit deny rules fail when unenforceable
   ([`sandbox/manager.go`](../../internal/sandbox/manager.go#L226-L290)).
3. **Central redaction.** Every registry result path is scrubbed before output
   budgeting/persistence ([`tools/registry.go`](../../internal/tools/registry.go#L190-L215)).
4. **Permission-bound extensions.** MCP tools default to prompt and persistent
   approval binds identity/autonomy
   ([`mcp/registry.go`](../../internal/mcp/registry.go#L260-L279),
   [`mcp/permissions.go`](../../internal/mcp/permissions.go#L216-L235)). Plugin
   `allow` is clamped unless explicitly enabled
   ([`plugins.go`](../../internal/plugins/plugins.go#L882-L901)).
5. **Atomic state publication.** Config, trust, credentials, OAuth, and permission
   stores use restrictive directories/files and replace complete temporary
   content rather than exposing partial writes.
6. **Promotion hardening.** Updater binary staging writes through an exclusively
   created handle and platform-specific promotion binds to that object
   ([`update/apply.go`](../../internal/update/apply.go#L258-L329)).
7. **Release workflow controls.** Actions are SHA pinned; package tests/checksums
   run before upload; npm publication uses OIDC/provenance and a reviewed
   environment. Release checkout credential persistence is the narrower SEC-09
   exception
   ([`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L33-L81),
   [`release-artifacts.yml`](../../.github/workflows/release-artifacts.yml#L115-L145)).
8. **Remote daemon boundary.** The bridge is opt-in TLS-only, refuses missing
   authentication, caps connection count/handshake time/bundle bytes, and closes
   unauthenticated peers
   ([`bridge.go`](../../internal/daemon/remote/bridge.go#L18-L117),
   [`bridge.go`](../../internal/daemon/remote/bridge.go#L156-L225)).
9. **Action output normalization.** The fixed multiline-output delimiter is not
   reachable from the current summary value: structured summaries collapse all
   whitespace and text summaries select one line before truncation
   ([`action-summary.mjs`](../../scripts/action-summary.mjs#L9-L55)). Preserve
   this invariant or adopt a random delimiter if multiline summaries are added.

## Priority order

1. Reproduce and fix SEC-01 and SEC-02.
2. Remove/deprecate literal daemon token argv input (SEC-08).
3. Introduce one rooted read/write/extraction primitive and use it for SEC-03/04.
4. Add updater limits (SEC-05) before expanding update-source flexibility.
5. Add independent release authenticity (SEC-06) and encrypted OAuth default
   (SEC-07) with migration/compatibility plans.
6. Disable persisted release checkout credentials (SEC-09), then resolve or
   formally dismiss DEP-01 based on helper reachability and upgrade testing.

See [Migration Plan](MIGRATION_PLAN.md) and the canonical acceptance criteria in
[Known Issues](KNOWN_ISSUES.md).
