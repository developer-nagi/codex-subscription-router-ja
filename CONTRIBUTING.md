# Contributing

## Development setup

Use Windows 11 x64 with Go 1.26+, Node.js 22.12+, npm, Python 3.11+, and the official
ChatGPT desktop app installed.

```powershell
npm ci --ignore-scripts
npm run check
npm run release:check
```

Never commit an app bundle, credentials, account state, or a capture containing an
unmasked email address, account name, device code, or installed plugin list.

## Patch changes

Renderer and main-process patches depend on exact upstream anchors. A change must:

1. Keep the official app immutable.
2. Fail closed when an expected anchor is absent.
3. Preserve account isolation and sticky thread ownership.
4. Keep control services on loopback with token authentication.
5. Add focused tests for backend behaviour, and a curated screenshot for a new
   user-visible state when appropriate.

Anchors depend on each official build's minifier output. Verify against the build
recorded in `docs/COMPATIBILITY.md`, and update that file in the same pull request when a
new official build needs different anchors.

The injected UI must not hardcode minified identifiers. Declare them as placeholders in
`ui/account-menu.js` and bind them in `RENDERER_BINDINGS`, so a missing declaration stops
the patch instead of crashing the renderer.

## Localization

Every user-facing string in the injected UI is a formatjs descriptor with an `id`, an
English `defaultMessage`, and a `description`. Adding a string means adding it to every
file in `ui/messages/`, because a missing key is caught by the consistency check.

Keep placeholders identical across languages. Use plain `{count}` interpolation in
translations rather than ICU plural categories; only the English `defaultMessage` uses
real plural forms, since a wrong plural category makes formatjs fail at runtime.

## Language

Code, comments, documentation, and commit messages are written in English. The Japanese
UI lives in `ui/messages/ja-JP.json`, alongside every other language.

PowerShell scripts containing non-ASCII characters need a UTF-8 BOM, because Windows
PowerShell 5.1 reads a BOM-less file as ANSI. `npm run check:powershell` enforces this.

## Taking changes from upstream

Upstream is the macOS build and has diverged substantially from this fork. Do not use
`git merge upstream/main`; review and apply commits individually. The procedure and the
decision log are in [UPSTREAM-SYNC.md](docs/UPSTREAM-SYNC.md).

```powershell
npm run upstream:check -- --fetch
```

## Pull requests

Keep changes focused and explain security-sensitive behaviour explicitly. CI checks Go
tests and vetting, JavaScript syntax, Python compilation, PowerShell syntax and BOM, and
release metadata consistency.
