# Compatibility

The patcher is deliberately tied to a known ChatGPT desktop layout. It verifies every
renderer and main-process anchor it rewrites and stops rather than applying a partial
patch.

## Release 0.1.0 (Windows)

| Component | Tested value |
| --- | --- |
| OS | Windows 11 x64 |
| Official package | MSIX `OpenAI.Codex` |
| Official version | `26.818.8289.0` |
| `app.asar` SHA-256 | `e2f04d6aa921d07981b42368df0a28a8bebe8cd21375d4a1f9286757b51c1313` |
| Electron fuse `EnableEmbeddedAsarIntegrityValidation` | Disabled, so repacking needs no hash update |
| Spawned app-server | `resources\codex.exe -c features.code_mode_host=true app-server --analytics-default-enabled` |

A different official version may work when every anchor is identical, but it is
unverified. The patcher rejects a version or ASAR hash mismatch by default;
`--allow-untested-source` is an explicit diagnostic override. Never weaken an anchor
count or a binary constant merely to make a new build complete. Review the upstream
change and update the patch deliberately.

## Verified patch anchors

Each one matches exactly once against official build `26.818.8289.0`.

| Target | Purpose |
| --- | --- |
| `connect-src` in `webview/index.html` | Let the CSP reach the control API |
| Profile menu component | Where the account menu is injected |
| `usageItems` slot | Renders the account list and pooled usage |
| `/wham/profiles/me` request | Swapped for combined profile statistics |
| Usage modal | Initializes the per-account reset state |
| Reset-credit query | Reads the selected account's remaining resets |
| Reset-credit mutation | Consumes a reset for the selected account only |
| Usage-window selection | Shows the selected account's allowance |
| Profile menu open-state hooks (2) | Keeps the menu open while a device code is visible |
| Depletion alerts (4) | Replaced with the pooled depletion message |
| Profile avatar, display name and username (3) | Show that the statistics are pooled |
| `setPath('userData')` in `bootstrap-*.js` | Isolates the desktop profile |
| Computer Use instruction in `main-*.js` | Replaced with the strict Windows wording |

## Renderer identifier bindings

The injected account UI references minified identifiers from the official build. Those
names change between builds, so `ui/account-menu.js` writes them as placeholders and the
patcher substitutes them only after confirming each declaration. It stops when one
cannot be confirmed.

| Placeholder | Identifier | Role | Declaration probe |
| --- | --- | --- | --- |
| `__CODEX_MUX_JSX__` | `u7` | JSX runtime | `u7=J()` |
| `__CODEX_MUX_REACT__` | `nql` | React (hooks) | `nql=r(s(),1)` |
| `__CODEX_MUX_MENU_ITEM__` | `fI` | Menu row component | `function fI(` |
| `__CODEX_MUX_MENU__` | `vI` | Menu namespace (`Separator`) | `vI={Trigger:` |
| `__CODEX_MUX_IMAGE_URL__` | `Ija` | Profile image URL resolution | `function Ija(` |
| `__CODEX_MUX_INTL__` | `pd` | `useIntl` for formatjs | `function pd()` |

Icons do not depend on official identifiers; the injected UI defines them as SVG.

## Localization

The injected UI uses the same mechanism as the official app: every string is a formatjs
descriptor with an `id`, a `defaultMessage` and a `description`. The official app looks
the id up in the active language's dictionary and falls back to `defaultMessage` when it
is missing.

`ui/messages/<locale>.json` holds the translations, and the patcher inserts them into the
matching official locale bundle. All 64 locales the app ships are covered, so the account
UI follows the display language exactly as the native strings do. English comes from
`defaultMessage` and needs no file.

Translations use plain `{count}` interpolation rather than ICU plural categories. Only
the English `defaultMessage` uses real plural forms, because a wrong plural category for
a language makes formatjs fail at runtime.

The depletion alerts are a special case. They reuse official message ids, and those ids
already have official translations in every language. formatjs prefers a translation over
`defaultMessage`, so the patcher also replaces the id with `codexMux.allSubscriptionsDepleted`.
Without that, the injected wording never appears in a translated language.

## Features not ported from the macOS build

The Windows build implements these screens differently, so the macOS anchors and
injection approach do not hold. The patch is simply not applied. Stating that plainly is
preferred over forcing an approximate anchor.

| Feature | State in the Windows build |
| --- | --- |
| Subscription switching on Settings → Plugins | The central app-server request bridge (`gm` on macOS) is gone, replaced by a `selectedHostId` architecture |
| Selecting one account's statistics on the Profile page | The pooled display works. There is no refetch handle in `profile-*.js` (macOS `refetch`), so the profile cannot be re-requested when the selection changes |
| The "Subscription" row in the thread summary | The section array in `local-conversation-thread-*.js` has a different shape, so the insertion point is not uniquely identifiable |
| Opening the usage modal from the "Usage remaining" row | The macOS modal opener (`BW`) does not exist, so the row is informational. Per-account reset selection still works once the modal opens natively |

Combined profile statistics themselves work. With two or more connections, the Profile
page shows the connected accounts as overlapping avatars along with "Combined profile"
and the connected count. A single connection keeps the native header.

Plugin definitions and MCP configuration are shared across accounts, so plugins are
usable from every subscription. Only the switching UI is missing.

`ui/thread-subscription.js` is kept for a future re-derivation. The patcher does not
inject it today.

## Relationship to the official plugin system

Official plugins ship as folders holding a `.codex-plugin/plugin.json`, and the personal
marketplace lives at `.agents/plugins/marketplace.json` under the user profile. Both sit
outside `CODEX_HOME`, so they are naturally shared between isolated accounts. That
matches this project's design of sharing plugin definitions and MCP configuration from
the Primary account, so installing plugins raises no problem. What is separated per
account is the connection (OAuth) state, and that switching UI is unavailable on Windows
as noted above.
