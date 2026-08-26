# Security model

## Trust boundaries

- The official ChatGPT desktop app is trusted build input and is never modified.
- The patcher has local filesystem access by design.
- Each Codex child is trusted with only its assigned account home.
- The injected renderer is trusted with the loopback control token.
- Processes running as the same Windows user are not considered isolated from one
  another; they can already read that user's app data.
- Other local users and remote origins are outside the control API boundary.

## Credentials

OAuth material stays in `auth.json` under each account's Codex home. The multiplexer
reads an account token only to call the same authenticated ChatGPT profile and
rate-limit-reset endpoints the desktop experience uses. It does not log tokens or return
them from the control API. The state it persists contains account paths, labels, enabled
state, and thread ownership only.

An imported `auth.json` is validated strictly before it is stored: a 64 KB size cap, a
single JSON value, no unknown fields, `auth_mode` of `chatgpt`, a null `OPENAI_API_KEY`,
and non-empty tokens. A duplicate ChatGPT account is rejected.

Windows does not implement POSIX permission bits. The `0o600` the Go code requests has no
effect, because `os.Chmod` only toggles the read-only attribute. Account homes and
`auth.json` are protected by the default permissions under `%USERPROFILE%` and by the ACL
applied when the state root is created.

That ACL is applied only when the state root is newly created. Rewriting an existing state
root would strip the permission the official app adds for its Windows sandbox. Applying an
ACL can require a privilege the process lacks; when it fails, the install continues, warns,
and relies on the default `%USERPROFILE%` permissions.

Plugin and MCP configuration is deliberately synchronized from the Primary account so
installed definitions stay consistent. Inline environment values inside those definitions
are therefore copied into every isolated account home. Account isolation is not a separate
secret boundary for shared plugin configuration.

## Network

The control server binds to `127.0.0.1`. Private endpoints require the token embedded into
the independently built local renderer. Profile images must use HTTPS. Response sizes and
JSON request bodies are bounded.

The project itself provides no telemetry or update endpoint. Traffic beyond loopback is
performed by the official Codex children or by the documented ChatGPT profile and
rate-limit APIs.

## Windows specifics

The copied app lives outside the MSIX package, so it has no package identity. A Store
update cannot overwrite the patched copy, but some OS integrations that depend on MSIX
identity may not behave exactly as they do for the official app.

The Windows build ships the Electron fuse `EnableEmbeddedAsarIntegrityValidation`
disabled, so a repacked `app.asar` is not verified. That is an upstream setting, not one
this project changes. `RunAsNode` and `EnableNodeOptionsEnvironmentVariable` are also
enabled upstream, so processes running as the same user can make use of the app's Node
runtime. The same is true of the official app.

Windows has no per-app privacy permission equivalent to macOS TCC, so there is no
separately signed Computer Use helper or permission row to manage, and no corresponding
patch.

## Diagnostics

`CODEX_MUX_UI_TESTS=1` enables deterministic preview and screenshot endpoints. They are
unavailable during a normal launch, bind only to loopback, and require the same control
token. Release workflows never set this variable.

## Distribution

Releases contain source only. Publishing the patched app, the official ASAR, or any
extracted OpenAI binary is outside this project's release process.
