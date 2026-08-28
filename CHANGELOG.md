# Changelog

This project follows [Keep a Changelog](https://keepachangelog.com/) and
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- Replaced the target platform: macOS becomes **Windows 11 x64**. The official app is
  detected automatically from the MSIX package `OpenAI.Codex`.
- Replaced the installer, `install.sh` becoming `install.ps1`.
- Replaced the launcher, the macOS C implementation becoming Go (`cmd/launcher`). It is
  built with `-H=windowsgui` so it passes the isolated profile without a console window.
- Moved the CI and release workflows to `windows-latest`, and added
  `npm run check:powershell` to check PowerShell syntax and the BOM requirement.
- Made English the default language of the project. Documentation, comments, and commit
  messages are English; Japanese lives in `ui/messages/ja-JP.json` like every other
  language.

### Added

- The multiplexer resolves `codex.real.exe` and can terminate its children on Windows.
- The injected UI declares build-specific identifiers as placeholders, and the patcher
  binds them only after verifying each declaration. It fails closed otherwise.
- Localization for the injected UI through the same formatjs mechanism the official app
  uses. Translations for all 64 shipped locales are inserted into the official locale
  bundles, so the account UI follows the display language exactly as native strings do.
  Subscription names are part of that: they are rendered from the interface language
  rather than from the label stored when the account was added, which would otherwise
  keep whichever language was active at that moment.
- A remaining-allowance bar under each subscription label, plus the account's banked
  reset credits beside the percentage as `♻0`. The bar runs blue while there is room,
  orange under 30 percent remaining, and red under 10; the reset count is green.
- The Profile page shows that its statistics are pooled. With two or more connections it
  renders the connected accounts as overlapping avatars with "Combined profile". The
  statistics were already pooled, but a single account's identity hid that.
- `CODEX_MUX_TRACE`. Set to a file path, it records the method names crossing the
  multiplexer, with a thread id, an error code and message, and a subscription id. It
  writes no prompt text, no results, and no credentials, and is off unless set. It is
  what established that a goal runs its own request rather than `turn/start`.
- Sharing a chat's history with another subscription. The app-server finds a chat by its
  id inside its own Codex home, so a chat recorded under one subscription is invisible
  to another however the path is passed. The receiving home is given a directory entry
  for the same file - a hard link, since a history reaches hundreds of megabytes and
  duplicating it during a handover is when that cost is least affordable - with a copy
  as the fallback. The app-server reports a history as an extended-length Windows path
  (`\\?\C:\...`), which is handled when the path is compared with its subscription's
  home.
- The composer names the subscription the open chat is running on, beside the chat's own
  controls. It names the subscription its next turn will be charged to, which is not
  always the one the chat belongs to.
- The offer to buy credits or invite a friend now waits until every connected
  subscription is out. The app decides to show it from the state of one subscription, so
  with several connected it appeared while the others still had most of their allowance.
- A process for taking upstream changes selectively. `npm run upstream:check` classifies
  unreviewed upstream commits, warns about branches cut from an older base, and lists the
  upstream's open pull requests, which mostly come from forks and never appear in
  `git ls-remote`. Decisions are recorded in `scripts/upstream-sync.json`, and the
  procedure is in `docs/UPSTREAM-SYNC.md`.
- Dependency updates proposed by upstream's Dependabot, applied individually:
  `@electron/asar` 4.2.1 → 4.3.0, `actions/checkout` v6 → v7.0.1, and
  `actions/setup-node` v6 → v7.0.0. Two of those upstream branches were cut before the
  project rename, so their content was applied rather than merged.

### Fixed

- The merged chat list no longer decides who owns a chat. Every listing wrote down the
  answering subscription as each chat's owner, and the subscriptions answer
  concurrently, so ownership rested on which reply happened to arrive last. It is now
  recorded only for a chat that has none.
- `error` notifications now reach the interface from every subscription. They were
  forwarded only for the Primary account, so a failure on any other subscription was
  silent.

### Removed

- `ui/thread-subscription.js`. It was written for the macOS build and never injected here;
  the composer footer names the chat's subscription instead.
- Moving a running chat to another subscription, and everything built for it: sharing the
  history file, carrying the goal across, and routing a chat's work away from the
  subscription that owns it. It cannot be done on this build, and attempting it damages
  the chat.

  The app-server finds a chat by its id inside its own Codex home, and a chat's turns are
  rebuilt into that home's own store by a writer attached to the loaded session - reading
  a chat never advances it. A subscription handed a chat therefore cannot show it until it
  has read the whole history, while the subscription that had been reading it stops at
  whatever it had read. The writer also demands the history's records be numbered without
  a break, and stops permanently and silently when they are not.

  Sharing one history file between two subscriptions corrupts the chat outright: each
  app-server keeps its own record numbering, so two writing to one file interleave two
  sequences into it. Measured on a real 804 MB chat, whose numbering breaks at 717 MB and
  which neither subscription can now display in full.

  Routing a NEW chat by quota needs no move and is unaffected.
- Code signing, Apple team identifier rewriting, the independently signed Computer Use
  helper, and `ElectronAsarIntegrity` updates. The Windows build ships the Electron fuse
  `EnableEmbeddedAsarIntegrityValidation` disabled and needs none of it.
- The macOS-only `install.sh` and `native/launcher.c`.

### Taken from upstream

- Routing requests billed outside a ChatGPT subscription away from the pool (upstream
  PR #13). An explicit model provider, a provider-prefixed model, or a non-OpenAI base
  URL is sent through the account's own Codex provider.
- Subscription removal (upstream PR #21). A removed account's Codex home moves to
  `backups/accounts` so the removal stays recoverable. Windows cannot move a directory a
  running process still holds, so removal waits for the child to exit and retries while
  handles are released.
- `auth.json` import (upstream PR #20), which adds a way to connect an account when
  device-code authentication is unavailable on it.
- Restored the CORS `Access-Control-Allow-Headers` that upstream PR #21 dropped, and
  added a preflight regression test. Without that header every request from the injected
  UI fails.
- Manage mode also lists accounts that never finished signing in, so one cannot be left
  behind with no way to clear it.

Those three pull requests are still open upstream, so they may need to follow later
changes.

### Not ported

- Subscription switching on Settings → Plugins, selecting one account's statistics on the
  Profile page, the "Subscription" row in the thread summary, and opening the usage modal
  from the "Usage remaining" row. The Windows build implements those screens differently.
  See `docs/COMPATIBILITY.md`.

## [0.1.0] - 2026-08-15

### Added

- Multi-subscription routing with quota-aware balancing and sticky threads.
- Account isolation, device-code sign-in, pooled usage, and depletion failover.
- A native account menu with masked emails, plan labels, and profile photos.
- Combined profile statistics with per-account selection.
- Per-account Apps and MCP connection state on Settings → Plugins.
- Per-account rate-limit reset selection and pool-wide depletion handling.
- Independently signed Appshots and Computer Use support.
- Fail-closed upstream compatibility checks and depth-first helper signing.
- Loopback-only, token-authenticated diagnostic UI states.
- Source-only CI, release draft automation, security documentation, and a smoke test.

[Unreleased]: https://github.com/developer-nagi/codex-subscription-router-win/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/developer-nagi/codex-subscription-router-win/releases/tag/v0.1.0
