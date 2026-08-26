#!/usr/bin/env python3
"""List new upstream changes and classify how this fork should treat them.

This fork replaced the macOS build with a Windows one, so `git merge upstream/main`
would conflict broadly. Upstream changes are applied one commit at a time instead.
This script only reports what is needed to decide; it does not change the repository
(apart from `--mark-reviewed`).
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parent.parent
STATE_PATH = Path(__file__).resolve().parent / "upstream-sync.json"

# Path classification by how this fork treats each area, matched by prefix.
NOT_APPLICABLE = (
    "install.sh",  # replaced by install.ps1
    "native/",  # replaced by the Go cmd/launcher
)
REWRITTEN = (
    "scripts/patch_app.py",  # rewritten for Windows
    "scripts/check_release.py",
    "README.md",
    "docs/",
    ".github/workflows/",
    "package.json",
    "CHANGELOG.md",
    "CONTRIBUTING.md",
    "SECURITY.md",
    "NOTICE.md",
)
SHARED = (
    "cmd/",
    "internal/",
    "ui/",
    "go.mod",
    "go.sum",
    "VERSION",
)

VERDICT_ORDER = ("candidate", "review", "not applicable", "unclassified")


def use_utf8_output() -> None:
    """Write UTF-8 even when the console defaults to a legacy code page."""
    for stream in (sys.stdout, sys.stderr):
        reconfigure = getattr(stream, "reconfigure", None)
        if reconfigure is not None:
            reconfigure(encoding="utf-8", errors="replace")


def run_git(*args: str) -> str:
    result = subprocess.run(
        ["git", *args],
        cwd=PROJECT_ROOT,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    return result.stdout.strip()


def load_state() -> dict:
    if not STATE_PATH.is_file():
        return {"lastReviewedUpstreamCommit": None, "decisions": []}
    return json.loads(STATE_PATH.read_text(encoding="utf-8"))


def save_state(state: dict) -> None:
    STATE_PATH.write_text(
        json.dumps(state, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )


def classify_path(path: str) -> str:
    if path.startswith(NOT_APPLICABLE):
        return "not applicable"
    if path.startswith(REWRITTEN):
        return "review"
    if path.startswith(SHARED):
        return "candidate"
    return "unclassified"


def classify_commit(paths: list[str]) -> str:
    verdicts = {classify_path(path) for path in paths}
    for verdict in VERDICT_ORDER:
        if verdict in verdicts:
            return verdict
    return "unclassified"


def report_commits(state: dict, upstream_ref: str) -> int:
    upstream_head = run_git("rev-parse", upstream_ref)
    reviewed = state.get("lastReviewedUpstreamCommit")

    print(f"upstream head: {upstream_head[:7]} ({upstream_ref})")
    print(f"last reviewed: {reviewed[:7] if reviewed else '(none)'}")

    if reviewed == upstream_head:
        print("\nNo new upstream commits.")
        return 0

    span = f"{reviewed}..{upstream_head}" if reviewed else upstream_head
    raw = run_git("log", "--reverse", "--format=%H%x1f%s", span)
    if not raw:
        print("\nNo new upstream commits.")
        return 0

    entries = [line.split("\x1f", 1) for line in raw.splitlines() if line]
    print(f"\nUnreviewed upstream commits: {len(entries)}\n")
    for commit, subject in entries:
        paths = run_git(
            "show", "--pretty=format:", "--name-only", commit
        ).splitlines()
        paths = [path for path in paths if path]
        verdict = classify_commit(paths)
        print(f"[{verdict}] {commit[:7]} {subject}")
        for path in paths[:12]:
            print(f"    {classify_path(path):<10} {path}")
        if len(paths) > 12:
            print(f"    … and {len(paths) - 12} more files")
        print()

    print("See docs/UPSTREAM-SYNC.md for how to apply these.")
    return 0


def report_stale_branches(upstream_ref: str) -> None:
    """Warn about upstream branches cut from a commit older than the head.

    Merging one as-is would revert the upstream changes made after that point.
    """
    upstream_head = run_git("rev-parse", upstream_ref)
    remote = upstream_ref.split("/", 1)[0]
    raw = run_git(
        "for-each-ref", "--format=%(refname:short)", f"refs/remotes/{remote}"
    )
    branches = [
        name
        for name in raw.splitlines()
        if name and name != upstream_ref and not name.endswith("/HEAD")
    ]
    if not branches:
        return

    stale = []
    for branch in branches:
        base = run_git("merge-base", upstream_ref, branch)
        if base != upstream_head:
            behind = run_git("rev-list", "--count", f"{base}..{upstream_head}")
            stale.append((branch, base, behind))

    if not stale:
        return

    print("\nUpstream branches cut from an older base (do not merge as-is):")
    for branch, base, behind in stale:
        print(f"  {branch}")
        print(f"    base {base[:7]} / {behind} commits behind the head")
    print("  Apply their content individually. See docs/UPSTREAM-SYNC.md.")


def upstream_repository(upstream_ref: str) -> str | None:
    """Extract `owner/repo` from the upstream remote URL."""
    remote = upstream_ref.split("/", 1)[0]
    try:
        url = run_git("remote", "get-url", remote)
    except subprocess.CalledProcessError:
        return None
    match = re.search(r"github\.com[:/]+([^/]+/[^/]+?)(?:\.git)?/?$", url)
    return match.group(1) if match else None


def report_open_pull_requests(upstream_ref: str) -> None:
    """List and classify the upstream's open pull requests.

    Most upstream pull requests come from contributor forks, so they never appear in
    `git ls-remote`. Looking only at branches misses them.
    """
    repository = upstream_repository(upstream_ref)
    if repository is None:
        return
    if shutil.which("gh") is None:
        print("\nOpen pull requests: skipped because gh is not installed.")
        return

    result = subprocess.run(
        [
            "gh", "pr", "list", "--repo", repository, "--state", "open",
            "--limit", "50", "--json", "number,title,headRefName,files",
        ],
        capture_output=True, text=True, encoding="utf-8", errors="replace",
    )
    if result.returncode != 0:
        print("\nOpen pull requests: skipped because gh failed to run.")
        return

    pulls = json.loads(result.stdout or "[]")
    if not pulls:
        print("\nNo open pull requests.")
        return

    print(f"\nOpen upstream pull requests: {len(pulls)} (unmerged; decide on each one)\n")
    for pull in pulls:
        paths = [entry["path"] for entry in pull.get("files") or []]
        verdict = classify_commit(paths) if paths else "unclassified"
        print(f"[{verdict}] #{pull['number']} {pull['title']}")
        shared = [path for path in paths if classify_path(path) == "candidate"]
        if shared:
            print(f"    shared with this fork: {', '.join(shared[:6])}")
        print()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--upstream", default="upstream/main", help="the upstream ref to compare against"
    )
    parser.add_argument(
        "--fetch", action="store_true", help="fetch the upstream remote first"
    )
    parser.add_argument(
        "--mark-reviewed",
        metavar="COMMIT",
        help="record a commit as reviewed; defaults to the upstream head.",
        nargs="?",
        const="",
    )
    args = parser.parse_args()
    use_utf8_output()

    try:
        if args.fetch:
            remote = args.upstream.split("/", 1)[0]
            print(f"Fetching {remote}…")
            run_git("fetch", remote, "--prune")

        state = load_state()

        if args.mark_reviewed is not None:
            commit = args.mark_reviewed or args.upstream
            resolved = run_git("rev-parse", commit)
            state["lastReviewedUpstreamCommit"] = resolved
            save_state(state)
            print(f"Recorded {resolved[:7]} as reviewed.")
            return 0

        report_commits(state, args.upstream)
        report_stale_branches(args.upstream)
        report_open_pull_requests(args.upstream)
    except subprocess.CalledProcessError as error:
        message = (error.stderr or "").strip() or error.stdout.strip()
        print(f"upstream check failed: {message}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
