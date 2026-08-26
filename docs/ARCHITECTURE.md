# Architecture

Codex Subscription Router replaces the copied app's `resources\codex.exe` with a small Go
multiplexer and keeps the original binary beside it as `codex.real.exe`.

The independent desktop uses `%APPDATA%\Codex Subscription Router`, separate from the
official app's `%APPDATA%\Codex`. Two mechanisms provide that separation: rewriting
Electron's `app.setPath('userData')`, and changing `UserDataDirectoryName` in the owl
shell's `resources\owl-app.ini`. The `.codex-mux` state directory is maintained
independently of the official app.

## Request routing

The desktop opens one JSON-RPC app-server connection to the multiplexer. The multiplexer
starts one real app-server child per enabled account, each with its own `CODEX_HOME` and
`CODEX_SQLITE_HOME`.

New threads are assigned by quota urgency: the weekly percentage remaining divided by the
hours until that account resets, plus a capped bonus for banked reset credits.
Short-window usage, the existing pinned-thread count, and a stable account order break
close results. Reset-credit metadata is fetched in parallel, cached for five minutes, and
treated as neutral when unavailable.

Once a thread ID is known, `state.json` persists its owner. Requests, responses,
approvals, and notifications are rewritten only as needed to preserve one coherent
desktop session.

If the owner is depleted, the multiplexer resumes the rollout on an account with capacity
and updates ownership. Threads do not migrate for ordinary load balancing.

Requests billed outside a ChatGPT subscription — an explicit model provider, a
provider-prefixed model, or a non-OpenAI base URL — skip the pool and go through the
account's own Codex provider.

## Passthrough

The multiplexer only intercepts an interactive `app-server` invocation. `app-server
daemon`, `app-server proxy`, schema generation, and every other subcommand are handed to
`codex.real.exe` unchanged, preserving the exit code. The real binary is resolved as
`codex.real.exe`, then `codex.real`.

## Account isolation

The Primary account uses `%USERPROFILE%\.codex`. Added accounts use
`%USERPROFILE%\.codex-mux\accounts\<id>\codex-home`. Managed configuration is copied from
the Primary account, excluding credential-store settings and project trust. Each isolated
account forces file-backed CLI and MCP OAuth credentials.

Official plugins and skills live under `.agents` in the user profile, outside
`CODEX_HOME`, so they are shared by every account.

Removing a subscription stops its app-server child, clears its thread ownership, and
moves its Codex home into `backups/accounts` so the removal stays recoverable. Windows
cannot move a directory a running process still holds, so the removal waits for the child
to exit and retries while handles are released.

## Desktop integration

The patcher extracts `app.asar`, verifies that each upstream anchor matches exactly once,
injects the account UI, and repacks the archive. The Windows build ships the Electron
fuse `EnableEmbeddedAsarIntegrityValidation` disabled, so neither an integrity hash update
nor code signing is required. Native modules (`@worklouder`, `better-sqlite3`,
`node-pty`) are kept unpacked and verified after repacking.

`CodexSubscriptionRouter.exe` is a Go launcher built with `-H=windowsgui` so no console
window appears. It passes the isolated profile as `--user-data-dir` before Electron's
main process starts.

Computer Use on Windows uses the Node runtime in `resources\cua_node` and the native
modules in `resources\native` directly. There is no separately signed helper app or
privacy-permission mechanism as on macOS, so there is no corresponding patch.

## Localization

The injected UI declares formatjs message descriptors, and the patcher inserts
translations from `ui/messages/<locale>.json` into the official locale bundles. The UI
therefore follows the app's display language through the same path as the native strings.
See [Compatibility](COMPATIBILITY.md) for details.

## Control API

The renderer talks to a loopback-only HTTP service on port 48123. Every private route
requires a random 256-bit token. CORS is limited to the copied app's `app://-` origin. The
service exposes account metadata, aggregated usage and profile data, thread ownership,
sign-in, sign-out, subscription import and removal, and an authenticated SSE event
stream. It never returns OAuth tokens.
