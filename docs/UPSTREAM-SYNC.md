# Taking changes from upstream

This fork replaced upstream's (`b-nnett/codex-subscription-router`) macOS build with a
Windows one. What the two still share is the Go multiplexing logic and part of the
injected UI; the patcher, installer, CI, and documentation are different work.

For that reason **`git merge upstream/main` is not used**. Upstream changes are reviewed
one commit at a time and only the relevant ones are applied.

## Principles

1. Never merge an upstream branch directly. Merging a branch cut from an older base
   reverts the upstream changes made after that point.
2. Record the decision and its reasoning in `scripts/upstream-sync.json` so the same
   review is not repeated.
3. After applying anything, run `npm run check`, and rebuild on real hardware when the
   change touches the patcher.

## Reviewing

```powershell
npm run upstream:check
```

To fetch the `upstream` remote first:

```powershell
npm run upstream:check -- --fetch
```

The output lists unreviewed upstream commits and classifies their files:

| Classification | Paths | Treatment |
| --- | --- | --- |
| candidate | `cmd/`, `internal/`, `ui/`, `go.mod`, `VERSION` | Shared with this fork; usually applies as-is |
| review | `scripts/`, `README.md`, `docs/`, `.github/workflows/`, root documents | Rewritten for Windows; read the diff and reimplement the intent |
| not applicable | `install.sh`, `native/` | Deleted in this fork; do not apply |
| unclassified | Anything else | Judge individually and update the rules in `scripts/check_upstream.py` |

Branches cut from a base older than the upstream head are listed as a warning, and the
upstream's open pull requests are listed and classified the same way. Most upstream pull
requests come from contributor forks, so they never appear in `git ls-remote`; looking
only at branches misses them.

## Applying

### Shared code (`cmd/`, `internal/`, `ui/`)

Start from a clean working tree and take the change without committing so it can be
reviewed.

```bash
git cherry-pick -n <commit>
```

Resolve conflicts while keeping this fork's Windows support in place. Take particular
care not to revert:

- The Go module path (`github.com/developer-nagi/codex-subscription-router-win`)
- Windows-specific behaviour (`internal/backend/terminate_windows.go`,
  `resolveRealExecutable`, the removal retry in `internal/mux/accounts.go`)
- The injected UI's placeholder bindings and formatjs message descriptors

Upstream writes the injected UI against the macOS build's minified identifiers. Rewrite
those to the placeholders this fork uses, and add any new user-facing string to
`ui/messages/*.json` for every locale.

### Rewritten areas (`scripts/`, `docs/`, workflows)

Do not cherry-pick these. Read the diff and take only the intent.

```bash
git show <commit> -- scripts/ docs/
```

Upstream changes to patcher anchors assume the macOS build. The Windows build uses
different identifiers, so never carry them over directly. Re-verify the anchor and
binding tables in `docs/COMPATIBILITY.md` against the real build.

### Dependency updates (Dependabot)

Do not merge an upstream Dependabot branch; apply the version change only.

```powershell
npm install --save-exact --ignore-scripts @electron/asar@<version>
```

GitHub Actions are pinned by commit SHA, so replace the SHA and always verify it.

```bash
gh api repos/actions/checkout/commits/<sha> --jq .sha
```

`@electron/asar` is central to the patcher (extract, pack, list), so rebuild on real
hardware after updating it.

## Recording

After deciding to apply or skip something, append to `decisions` in
`scripts/upstream-sync.json` and move the reviewed marker forward.

```powershell
npm run upstream:check -- --mark-reviewed
```

To mark a specific commit instead:

```powershell
npm run upstream:check -- --mark-reviewed <commit>
```

## Verifying

```powershell
npm run check
npm run release:check
```

When the patcher, the injected UI, or the Go multiplexing logic changed, rebuild and
launch on real hardware. The procedure is in [SMOKE-TEST.md](SMOKE-TEST.md).

```powershell
python scripts/patch_app.py --force
```

## Decisions so far

See `scripts/upstream-sync.json`. As of 2026-08-27, upstream `main` has no commits after
this fork diverged. Of the three Dependabot branches, two were cut three commits behind
the head and were not merged; their content was applied instead. Three open pull requests
were taken (#13, #20, #21) and the rest were judged not applicable.
