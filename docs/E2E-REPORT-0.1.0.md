# End-to-end verification report — Windows port

Commit: Windows port working branch
Environment: Windows 11 Pro 10.0.26200 (x64)
Official app: MSIX `OpenAI.Codex` `26.818.8289.0`
`app.asar` SHA-256: `e2f04d6aa921d07981b42368df0a28a8bebe8cd21375d4a1f9286757b51c1313`

## Feasibility checks

| Check | Result |
| --- | --- |
| What the official app actually is | An MSIX package containing an Electron app with a 287 MB `app.asar` |
| Copying it out of the MSIX and launching | Succeeded: a window appeared with 11 child processes |
| Code signing and package identity | Not required; the copied app launches as-is |
| Electron fuse `EnableEmbeddedAsarIntegrityValidation` | Disabled, so repacking needs no hash update |
| Spawned app-server | `resources\codex.exe -c features.code_mode_host=true app-server --analytics-default-enabled` |

## Patch anchors

Every anchor listed in `docs/COMPATIBILITY.md` matches the official build exactly once,
verified by an automated check.

## Build and install

`python scripts/patch_app.py` completed with exit code 0 and produced:

- `%LOCALAPPDATA%\Programs\Codex Subscription Router`
- A Start Menu shortcut

State after installing:

| Check | Result |
| --- | --- |
| `resources\codex.exe` | Replaced with the multiplexer (7.2 MB) |
| `resources\codex.real.exe` | The official binary (297 MB) is preserved |
| `resources\codex.exe --help` | Printed the official CLI help, confirming passthrough |
| `UserDataDirectoryName` in `owl-app.ini` | `Codex Subscription Router` |
| `app.asar.unpacked\node_modules` | `@worklouder`, `better-sqlite3`, `node-pty` preserved |
| Official `app.asar` hash | Identical to before patching; the official app is unmodified |

## Runtime behaviour

| Check | Result |
| --- | --- |
| Launching from the launcher | Succeeded: 12 processes, window visible |
| Multiplexer start | Started as `resources\codex.exe ... app-server` |
| Real app-server child | Started `codex.real.exe ... app-server` |
| `/v1/health` | `200 {"ok":true}` |
| `/v1/accounts` with token | Returned the connected accounts with `planLabel` |
| Renderer diagnostics | No non-trivial errors |
| Thread ownership | Persisted in `state.json` |
| Effect on the official app | The official app kept running normally alongside it |

## Multi-subscription behaviour

Verified with two connected subscriptions:

| Check | Result |
| --- | --- |
| Connected accounts | 2, both reporting plan and rate limits |
| Combined profile | Aggregated across both accounts; the pooled total matched the sum |
| Combined profile header | Overlapping avatars, "Combined profile", and the connected count |
| Subscription removal | Removed a non-Primary account; its Codex home moved to `backups/accounts` |
| Removal on Windows | Waits for the child to exit and retries the rename while handles release |

## Localization

| Check | Result |
| --- | --- |
| Locales covered | 64 of 64, inserted into the official locale bundles |
| Messages | 47 per locale |
| Rendering in Japanese | Correct throughout the injected UI |
| `MISSING_TRANSLATION` for `codexMux.*` | None |

## Not verified

- Routing, failover, and per-account resets under sustained real usage across both
  accounts.
- The features recorded as not ported in `docs/COMPATIBILITY.md`.
- Locales other than English and Japanese were not visually inspected; their translations
  are machine-generated and have not been reviewed by native speakers.
