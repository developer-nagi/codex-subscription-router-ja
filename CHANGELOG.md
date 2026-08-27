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

- Failover when a subscription runs out mid-turn. `turn/start` is answered as soon as
  the turn is accepted, so a usage limit reached while the turn runs never appears in
  that response: the app-server reports it afterwards through an `error` notification.
  Only the response was inspected, so the chat stopped at the limit instead of moving
  on. Turns a subscription has accepted are now tracked, the notification is acted on,
  and the chat continues on another subscription. If none has capacity, the original
  error is released rather than swallowed.
- Setting a thread's goal is a turn too. It runs through its own request rather than
  `turn/start`, so it was neither checked against the subscription's remaining
  allowance nor eligible for failover, and a goal on a depleted subscription simply
  stopped.
- The capacity check in front of a turn no longer holds the turn up. It waited up to a
  minute for the subscription to answer, so a slow subscription froze the interface
  before the request was even sent. It now has a short budget and falls through to the
  owner, leaving the mid-turn failover to catch a limit reached anyway.
- Resuming a chat on another subscription no longer reads its whole history. Only the
  thread's identity and location are used, but every turn was being read, which on a
  long chat took long enough to look like the app had stopped.
- `error` notifications now reach the interface from every subscription. They were
  forwarded only for the Primary account, so a failure on any other subscription was
  silent.

### Removed

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
