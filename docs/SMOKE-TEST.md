# Smoke test

Complete this checklist on the exact official build recorded in
`docs/COMPATIBILITY.md` before publishing a release draft.

## Build and identity

- The patcher reports the expected official version and `app.asar` SHA-256.
- The official MSIX package is unchanged: its `app.asar` hash still matches.
- `resources\codex.exe` is the multiplexer and `resources\codex.real.exe` is the official
  binary.
- `resources\codex.exe --help` prints the official CLI help, confirming passthrough.
- `UserDataDirectoryName` in `resources\owl-app.ini` is the independent name.
- `app.asar.unpacked\node_modules` still contains `@worklouder`, `better-sqlite3`, and
  `node-pty`.

## Launch and routing

- The Start Menu shortcut launches the app and a window appears.
- `resources\codex.exe ... app-server` starts, and one `codex.real.exe` child starts per
  account.
- `http://127.0.0.1:48123/v1/health` returns `{"ok":true}`.
- `/v1/accounts` with the control token lists the connected accounts.
- Connect two or more subscriptions and confirm photos, plans, masked emails, pooled
  usage, and loading states.
- Start chats until each account has received one; confirm every follow-up stays on its
  original account.
- Spoof one depleted account and confirm the thread continues on an account with quota.
  Spoof all accounts depleted and confirm the combined alert.
- Open a quota-triggered reset sheet, switch subscriptions, consume a reset, and confirm
  only the selected account changes.

## Subscriptions

- Add a subscription with a device code, and add one by importing an `auth.json`.
- In Manage subscriptions, remove a non-Primary account and confirm its Codex home moved
  to `%USERPROFILE%\.codex-mux\backups\accounts`.
- Confirm the Primary subscription cannot be removed.
- Confirm an account whose sign-in never finished is listed while managing, so it can be
  cleared.

## Localization

- Run the app in English and in Japanese and confirm the injected rows follow the display
  language.
- Confirm that no `MISSING_TRANSLATION` warning names a `codexMux.*` id.

## Coexistence and rebuild

- The official ChatGPT desktop runs at the same time and both behave independently.
- The official profile at `%APPDATA%\Codex` is unchanged.
- Rebuild with `--force`: the previous installation moves to
  `%USERPROFILE%\.codex-mux\backups`, and account state and thread ownership survive.

Record the tested commit, the Windows version, the official app version, and any
deviations in the release draft before publishing it.
