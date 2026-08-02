# Update Flow

`zero update --check` checks the latest GitHub release and compares it with the
local CLI version.

```bash
zero update --check
zero update --check --json
zero update --check --repo Gitlawb/zero
zero update --check --target windows-x64
```

The command is intentionally check-only:

- It does not replace the running binary.
- It exits with code `0` when the check succeeds, even when an update is
  available.
- It exits with code `1` when the release check cannot be completed.
- `--json` prints the same result in a machine-readable format for scripts and
  CI.

Useful flags:

| Flag | Purpose |
|---|---|
| `--repo <owner/repo>` | Check another GitHub repository. |
| `--endpoint <url|owner/repo>` | Check a specific release API URL or repository slug. |
| `--timeout <duration>` | Override the default release check timeout. |
| `--target <platform-arch>` | Validate release metadata for another supported target. |

Supported targets are `linux-x64`, `linux-arm64`, `macos-x64`, `macos-arm64`,
`windows-x64`, and `windows-arm64`. Without `--target`, Zero checks the current
platform.

Endpoint resolution order:

1. `--endpoint`
2. `ZERO_UPDATE_RELEASE_URL`
3. `--repo`
4. `https://api.github.com/repos/Gitlawb/zero/releases/latest`

Installer scripts download the matching release asset for the local platform and
verify its `.sha256` file. If Zero is already installed, run `zero update --check`
before reinstalling.

## Windows recovery state (standalone installs)

This section describes Windows only. On Linux and macOS a standalone update
renames the staged file directly over the executable path through the
installation directory's file descriptor, so the replacement is atomic and no
aside copy, marker, or recovery record is ever created — a failed update leaves
the previous binary in place and nothing to resolve.

On Windows, a running executable cannot be replaced in place, so the previous
binary is moved aside to `<binary>.zero-update-<random>.old` first. The updater
records that exact file — bound to its filesystem identity, in per-user state
outside the installation directory — and deletes only that recorded copy after
the next update is verified in place. Backups it did not create are never
removed, so a file such as `zero.exe.before-manual-patch.old` is left alone. A
recorded copy that something else holds open (an on-access scanner, an editor)
stays recorded and is removed by a later update instead.

An update **refuses to run** while unresolved recovery state exists beside the
binary:

| State on disk | Meaning |
|---|---|
| `<binary>.old` (or `<binary>.<suffix>.old`) plus a `.keep` marker | A previous update could not restore the original binary. The `.old` file may be the last binary the updater verified; the installed one may be unverified. |
| `<binary>.…old.<suffix>.recovery` | A previous update could not even write the marker, so it moved the last verified binary to that name. |
| The binary is missing and one or more `*.old` files exist | The previous attempt was interrupted between moves. |

The refusal names the paths involved and the two moves that end the state:
either move the recovery binary back over the executable path, or — if the
installed binary is the one you want — delete the `.keep` marker (or the
`.recovery` copy, once you have verified the installed binary) and update again.

This is deliberately fail-closed and differs from older releases, which deleted
`<binary>.old` and proceeded. The trade-off is that anyone who can write in the
installation directory can plant `<binary>.old` and `<binary>.old.keep` there
and make every subsequent update refuse until an operator clears them. That is
the safe direction: the alternative is an update that overwrites the only
verified copy of the previous binary. If updates start refusing, inspect those
files before removing them — an installation directory that a lower-privileged
account can write to is itself worth fixing.
