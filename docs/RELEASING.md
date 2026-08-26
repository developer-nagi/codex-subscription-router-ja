# Releasing

Releases are source-only. Never attach a patched app, an ASAR, an extracted official
file, or account data.

1. Update `VERSION`, `package.json`, and both version fields in `package-lock.json`.
2. Move changelog entries from Unreleased into `## [x.y.z] - YYYY-MM-DD`.
3. Record the tested official app version and `app.asar` hash in `docs/COMPATIBILITY.md`.
4. Run `npm ci --ignore-scripts`, `npm run check`, and `npm run release:check` on Windows.
5. Complete `docs/SMOKE-TEST.md` and record the exact commit and Windows version in the
   release draft.
6. Review `git diff --check` and confirm no credentials or app bundles are staged.
7. Configure the protected `release` environment, tag the reviewed commit as `vX.Y.Z`,
   and push the tag.

The release workflow verifies that the tag matches `VERSION`, repeats every check, and
creates a source-only GitHub release draft with generated notes. Review the draft and the
smoke-test record before publishing it manually.
