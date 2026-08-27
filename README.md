# Codex Subscription Router (Windows Port)

![Multi-subscription account menu](screenshots/account-menu.png)

Use several ChatGPT subscriptions from one independent Windows desktop app.

Codex Subscription Router creates a locally patched copy of the official ChatGPT desktop
app and spreads new chats across the connected subscriptions. Each thread stays pinned to
one subscription, so follow-up turns keep their conversation context and benefit from
account-level caching.

The official app is read as build input and never modified. This repository contains
source code and build tooling only — no OpenAI binaries and no distributable app.

This repository is a fork of
[b-nnett/codex-subscription-router](https://github.com/b-nnett/codex-subscription-router).
The original was designed and built for macOS; this fork ports it to Windows. The core
design is upstream's work — see [Acknowledgements](#acknowledgements). On macOS, use the
original instead.

> [!WARNING]
> This is an unofficial project and is tightly coupled to a specific official build. It
> is not affiliated with OpenAI and carries no support. Review the source before using
> it, and judge for yourself whether your use complies with the terms governing each
> connected subscription.

![Combined profile](screenshots/combined-profile-20px.png)

## Highlights

- **Quota-aware routing.** New chats favour the weekly allowance that expires sooner,
  with a bounded boost for accounts holding banked usage resets.
- **Sticky threads.** Once assigned, a thread returns to the same subscription unless
  that subscription is depleted.
- **Automatic failover.** A depleted thread continues through another account with
  capacity. When the whole pool is empty, one combined alert is shown.
- **Continuing a running chat.** When a subscription runs out mid-turn, the turn moves to
  one with room and the chat carries on, goal included. The chat itself stays where it
  is; see [Continuing a chat when a subscription runs out](#continuing-a-chat-when-a-subscription-runs-out).
- **Which subscription is answering.** The composer names the subscription the open chat
  is running on, and its remaining allowance.
- **Quiet upsell.** The offer to buy credits or invite a friend waits until every
  connected subscription is out, rather than appearing whenever one of them is.
- **Native account management.** The existing profile menu shows pooled usage, profile
  photos, plan names, masked email addresses, and device-code sign-in.
- **Per-account resets.** The native rate-limit sheet shows and consumes reset credits
  for the selected subscription.
- **Combined profile.** Profile statistics are pooled across every subscription, with the
  connected accounts shown as overlapping avatars.
- **Adding and removing subscriptions.** Connect with a device code or by importing an
  existing `auth.json`. A removed account's data moves to a recoverable backup.
- **Localized interface.** The injected UI ships in all 64 languages the official app
  supports and follows the display language, exactly as the native strings do.

## How it works

The patched desktop still opens a single app-server connection. A small Go multiplexer
fans that connection out to one official Codex child per account. Each child has an
isolated Codex home, and the multiplexer records the owner of every thread.

```text
Codex Subscription Router (CodexSubscriptionRouter.exe)
        │
        │ one app-server connection
        ▼
    resources\codex.exe  (Go multiplexer)
    ├── Primary          → %USERPROFILE%\.codex
    ├── Subscription 2   → isolated Codex home
    └── Subscription 3   → isolated Codex home
             │
             └── thread ID → persistent account owner
```

The official `codex.exe` is kept beside it as `codex.real.exe` and started as a child of
the multiplexer. Anything that is not an `app-server` invocation is passed straight
through.

Read [the architecture](docs/ARCHITECTURE.md) and
[the security model](docs/SECURITY-MODEL.md) for detail.

## Compatibility

| Component | Supported value |
| --- | --- |
| OS | Windows 11 (x64) |
| Official ChatGPT desktop | MSIX package `OpenAI.Codex` `26.818.8289.0` |
| Go | 1.26 or newer |
| Node.js | 22.12 or newer |
| Python | 3.11 or newer |

The patcher verifies the official version, the `app.asar` SHA-256, and every anchor it
rewrites. An unknown upstream build is rejected by default rather than being partially
patched. The recorded hash is in [Compatibility](docs/COMPATIBILITY.md).

Unlike the macOS build, the Windows build needs neither code signing nor an ASAR
integrity hash update. The official build ships the Electron fuse
`EnableEmbeddedAsarIntegrityValidation` disabled, so a repacked `app.asar` loads as-is.

## Requirements

- The official ChatGPT desktop app from the Microsoft Store
- Go 1.26+
- Node.js 22.12+ and npm
- Python 3.11+

```powershell
winget install --id Git.Git
winget install --id GoLang.Go
winget install --id OpenJS.NodeJS.LTS
winget install --id Python.Python.3.12
```

## Install

Run this in PowerShell. It fetches or updates the source, installs the locked build
dependencies, creates the independent app, and launches it.

```powershell
irm https://raw.githubusercontent.com/developer-nagi/codex-subscription-router-win/main/install.ps1 | iex
```

The installer keeps its source checkout in
`%USERPROFILE%\.codex-subscription-router\source`. On an existing installation it reuses
the same account state and creates a recoverable backup. When a prerequisite or an
upstream compatibility check fails, it stops with a clear message instead of leaving a
partial installation.

> [!TIP]
> To inspect the installer first, open [`install.ps1`](install.ps1) or download it without
> piping it into a shell.

### Install from a clone

```powershell
git clone https://github.com/developer-nagi/codex-subscription-router-win.git
cd codex-subscription-router-win
npm ci --ignore-scripts
python scripts/patch_app.py
```

This creates:

- `%LOCALAPPDATA%\Programs\Codex Subscription Router`
- A Start Menu shortcut named "Codex Subscription Router"
- An independent desktop profile under `%APPDATA%\Codex Subscription Router`

It runs alongside the official app. Their profiles and user data are separate, so neither
affects the other.

## Add subscriptions

1. Open the profile menu at the bottom of the sidebar.
2. Select **Add another subscription** and choose how to connect.
   - **Continue with ChatGPT** — sign in from a browser with the displayed device code.
     Codex device-code authentication must be enabled on the account being added.
   - **Import auth.json** — read an existing Codex login file. Use this when device-code
     authentication is unavailable.
3. Return to the app and wait for the account row to appear.

While the code is visible, clicking away does not dismiss the menu. Clicking the code
copies it and opens the verification page.

The profile menu shows combined weekly usage followed by one row per subscription. Email
addresses stay masked until hovered. The final row always starts another sign-in.

Select **Manage subscriptions** to turn each row into a remove action. The Primary
subscription cannot be removed. Removing one takes its chats out of the app immediately,
but its account data moves to `%USERPROFILE%\.codex-mux\backups\accounts`, so it stays
recoverable. An account whose sign-in never finished is also listed while managing, so it
can be cleared from there.

## Routing behaviour

| Situation | Behaviour |
| --- | --- |
| New chat | Assigned by quota at risk, banked resets, and short-window pressure |
| Follow-up | Sent to the thread's persisted account owner |
| Owner depleted | Continued through another account with capacity |
| Owner depleted mid-turn | The turn moves to an account with room; the chat stays put |
| Every account depleted | One combined alert with the next known reset |
| Account disabled | Excluded from routing and pooled usable quota |
| Custom provider or non-OpenAI base URL | Sent through the account's configured Codex provider without using pooled ChatGPT quota |

## Continuing a chat when a subscription runs out

A new chat simply starts on a subscription that has room. A chat already running is
harder, because a chat is more than its history.

`turn/start` is answered as soon as the subscription accepts the turn, so a limit reached
while the turn runs never appears in that response - it arrives afterwards, as an `error`
notification. Setting a thread's goal runs a turn too, through a request of its own.
Both are recognised, and the turn is replayed on a subscription with room.

**The turn moves; the chat does not.** A chat's turns are rebuilt into whichever
subscription's own store, from a history that is read as the chat is used, and on a long
chat that rebuilding is nowhere near finished. A subscription just given the history
therefore cannot show the chat yet, and a chat handed over opened empty. The history file
is shared rather than copied, so whatever the running subscription appends is visible to
the one that owns the chat: reading stays where it works, and only the work moves.

The chat's goal is carried across before the handover resumes it, which is what keeps a
running goal running. A goal stopped because its subscription ran out is carried across
as running, since the reason it stopped does not apply where it is going.

This is behind an opt-in while it is proven out:

```powershell
$env:CODEX_MUX_THREAD_HANDOVER = "1"
```

Without it a chat whose subscription is out reports the limit, exactly as the official
app does, and nothing moves.

## Update and rebuild

The copied app is detached from the official MSIX package, so a Store update cannot
overwrite it. After updating the official app, confirm the new build is listed as
compatible, then rebuild.

```powershell
python scripts/patch_app.py --force
```

Quit Codex Subscription Router first. The existing installation moves to a timestamped
directory under `%USERPROFILE%\.codex-mux\backups`. Account state and credentials live
outside the app itself and are preserved. Delete old backups manually once the rebuilt app
passes the smoke test.

## Local data and security

| Path | Purpose |
| --- | --- |
| `%USERPROFILE%\.codex` | Primary credentials, conversations, and cache |
| `%USERPROFILE%\.codex-mux\state.json` | Account metadata and sticky thread ownership |
| `%USERPROFILE%\.codex-mux\accounts\<id>\codex-home` | Isolated secondary account data |
| `%USERPROFILE%\.codex-mux\control-token` | Token for the loopback-only control service |
| `%USERPROFILE%\.codex-mux\backups` | Recoverable app and account backups |
| `%APPDATA%\Codex Subscription Router` | Independent desktop profile |

The control service binds only to `127.0.0.1` and protects private routes with a random
256-bit token. OAuth tokens stay inside their account's Codex home and are never returned
by the control API. The state directory's ACL breaks inheritance and is limited to the
current user.

Plugin configuration is deliberately synchronized from the Primary account. Inline secrets
inside shared MCP configuration are therefore copied into each isolated account home; the
account directories are not a separate secret boundary.

Read [SECURITY.md](SECURITY.md) before reporting a credential, signing, or local
control-service issue.

## Development and verification

```powershell
npm ci --ignore-scripts
npm run check
npm run release:check
```

The Go backend and the injected renderer have no runtime third-party dependencies;
`@electron/asar` is build-only. Deterministic UI preview routes are enabled only when
`CODEX_MUX_UI_TESTS=1` is present at launch and always require the control token.

The on-device procedure is in [SMOKE-TEST.md](docs/SMOKE-TEST.md).

The multiplexer is silent by design, which makes a routing question hard to answer from a
running installation. Set `CODEX_MUX_TRACE` to a file path and it records the method
names crossing it, with a thread id, an error code and message, and a subscription id. It
writes no prompt text, no results, and no credentials, and is off unless set.

```powershell
$env:CODEX_MUX_TRACE = "$env:TEMP\codex-mux-trace.log"
```

## Known limitations

- An upstream ChatGPT update can require re-deriving the renderer anchors. They depend on
  each build's minifier output and are not compatible across versions.
- Some features from the macOS build are not ported, because the Windows build implements
  those screens differently: selecting one account's statistics on the Profile page, and
  subscription switching on Settings → Plugins. The combined profile display itself
  works, and the composer names the subscription in place of the macOS build's
  "Subscription" row in the thread summary. See [Compatibility](docs/COMPATIBILITY.md).
- Continuing a running chat on another subscription is opt-in, and the chat stays with
  the subscription that can display it while its turns run elsewhere.
- The initial merged history fetch is limited to 500 threads per account.
- Combined "skills explored" totals can count the same skill once per account, because
  the upstream profile response exposes counts rather than skill IDs.
- Translations other than English and Japanese are machine-generated and have not been
  reviewed by native speakers. Corrections go in `ui/messages/<locale>.json`.
- Releases are source-only; patched OpenAI binaries are never distributed.

## Taking changes from upstream

This fork has diverged substantially from upstream (the macOS build), so
`git merge upstream/main` is not used. Upstream changes are reviewed one commit at a time
and only the relevant ones are applied.

```powershell
npm run upstream:check -- --fetch
```

That lists unreviewed upstream commits, classifies their files as candidate, review, or
not applicable, and warns about branches cut from an older base. The procedure and the
decision log are in [UPSTREAM-SYNC.md](docs/UPSTREAM-SYNC.md) and
`scripts/upstream-sync.json`.

## Contributing and releases

Read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting changes. Releases follow the
source-only process in [RELEASING.md](docs/RELEASING.md) and require an on-device check
of the exact tagged commit.

## Acknowledgements

This project starts from
[codex-subscription-router](https://github.com/b-nnett/codex-subscription-router) by
Bennett Blackham. The following design and implementation are upstream's work, and this
fork inherits them:

- The multiplexing architecture that fans one app-server connection out to one Codex
  child per account.
- Assigning new chats by how much weekly allowance is at risk before it resets, with a
  bounded boost for banked usage resets.
- Pinning a thread to one subscription and handing it over only once that subscription is
  depleted.
- Isolating a Codex home per account while sharing only managed configuration.
- The fail-closed patching policy that stops when a single expected anchor is missing.
- The loopback-only, token-authenticated control API and the security model around it.

What this fork adds is replacing the platform with Windows and localizing the interface;
none of the thinking above was changed. Thank you for publishing such a solid foundation.

Upstream continues as the macOS build. On macOS, use
[the original](https://github.com/b-nnett/codex-subscription-router) rather than this
fork, and please do not bring issues or requests from this port upstream.

## License

This project's source is available under the [MIT License](LICENSE). The copyright notice
is kept as the original author's, Bennett Blackham.

ChatGPT, Codex, and the official Windows app are OpenAI products and are not covered by
this license.
