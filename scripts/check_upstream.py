#!/usr/bin/env python3
"""fork 元の新しい変更を洗い出し、取り込み方針を分類する。

本 fork は macOS 版から Windows 版へ全面的に置き換えており、`git merge upstream/main`
は広範な衝突を生む。上流の変更はコミット単位で選んで適用する。このスクリプトは
その判断材料だけを出力し、リポジトリの状態は変更しない (`--mark-reviewed` を除く)。
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

# 本 fork での扱いに応じたパス分類。前方一致で判定する。
NOT_APPLICABLE = (
    "install.sh",  # install.ps1 へ置き換え済み
    "native/",  # cmd/launcher の Go 実装へ置き換え済み
)
REWRITTEN = (
    "scripts/patch_app.py",  # Windows 専用に全面書き換え
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

VERDICT_ORDER = ("取り込み候補", "要注意", "対象外", "分類なし")


def use_utf8_output() -> None:
    """既定が cp932 のコンソールでも日本語を出力できるようにする。"""
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
        return "対象外"
    if path.startswith(REWRITTEN):
        return "要注意"
    if path.startswith(SHARED):
        return "取り込み候補"
    return "分類なし"


def classify_commit(paths: list[str]) -> str:
    verdicts = {classify_path(path) for path in paths}
    for verdict in VERDICT_ORDER:
        if verdict in verdicts:
            return verdict
    return "分類なし"


def report_commits(state: dict, upstream_ref: str) -> int:
    upstream_head = run_git("rev-parse", upstream_ref)
    reviewed = state.get("lastReviewedUpstreamCommit")

    print(f"upstream の先端: {upstream_head[:7]} ({upstream_ref})")
    print(f"確認済みコミット: {reviewed[:7] if reviewed else '(未設定)'}")

    if reviewed == upstream_head:
        print("\n新しい上流コミットはない。")
        return 0

    span = f"{reviewed}..{upstream_head}" if reviewed else upstream_head
    raw = run_git("log", "--reverse", "--format=%H%x1f%s", span)
    if not raw:
        print("\n新しい上流コミットはない。")
        return 0

    entries = [line.split("\x1f", 1) for line in raw.splitlines() if line]
    print(f"\n未確認の上流コミット: {len(entries)} 件\n")
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
            print(f"    … 他 {len(paths) - 12} ファイル")
        print()

    print("適用手順は docs/UPSTREAM-SYNC.md を参照する。")
    return 0


def report_stale_branches(upstream_ref: str) -> None:
    """上流ブランチのうち、先端より前から分岐しているものを警告する。

    そのまま merge すると、分岐後に入った上流の変更を巻き戻してしまう。
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

    print("\n分岐元が古い上流ブランチ (そのまま merge しない):")
    for branch, base, behind in stale:
        print(f"  {branch}")
        print(f"    分岐元 {base[:7]} / 先端より {behind} コミット前")
    print("  内容だけを個別に適用する。docs/UPSTREAM-SYNC.md を参照。")


def upstream_repository(upstream_ref: str) -> str | None:
    """upstream remote の URL から `owner/repo` を取り出す。"""
    remote = upstream_ref.split("/", 1)[0]
    try:
        url = run_git("remote", "get-url", remote)
    except subprocess.CalledProcessError:
        return None
    match = re.search(r"github\.com[:/]+([^/]+/[^/]+?)(?:\.git)?/?$", url)
    return match.group(1) if match else None


def report_open_pull_requests(upstream_ref: str) -> None:
    """上流のオープンな PR を分類して示す。

    上流の PR はフォークのブランチから出ていることが多く、`git ls-remote` には
    現れない。ブランチだけを見ていると変更を取りこぼす。
    """
    repository = upstream_repository(upstream_ref)
    if repository is None:
        return
    if shutil.which("gh") is None:
        print("\nオープンな PR: gh が無いため確認を省略した。")
        return

    result = subprocess.run(
        [
            "gh", "pr", "list", "--repo", repository, "--state", "open",
            "--limit", "50", "--json", "number,title,headRefName,files",
        ],
        capture_output=True, text=True, encoding="utf-8", errors="replace",
    )
    if result.returncode != 0:
        print("\nオープンな PR: gh の実行に失敗したため確認を省略した。")
        return

    pulls = json.loads(result.stdout or "[]")
    if not pulls:
        print("\nオープンな PR はない。")
        return

    print(f"\n上流のオープンな PR: {len(pulls)} 件 (未マージのため適用は個別に判断する)\n")
    for pull in pulls:
        paths = [entry["path"] for entry in pull.get("files") or []]
        verdict = classify_commit(paths) if paths else "分類なし"
        print(f"[{verdict}] #{pull['number']} {pull['title']}")
        shared = [path for path in paths if classify_path(path) == "取り込み候補"]
        if shared:
            print(f"    共有部分: {', '.join(shared[:6])}")
        print()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--upstream", default="upstream/main", help="比較対象の上流参照"
    )
    parser.add_argument(
        "--fetch", action="store_true", help="実行前に upstream を fetch する"
    )
    parser.add_argument(
        "--mark-reviewed",
        metavar="COMMIT",
        help="確認済みコミットを記録する。既定は上流の先端。",
        nargs="?",
        const="",
    )
    args = parser.parse_args()
    use_utf8_output()

    try:
        if args.fetch:
            remote = args.upstream.split("/", 1)[0]
            print(f"{remote} を fetch 中…")
            run_git("fetch", remote, "--prune")

        state = load_state()

        if args.mark_reviewed is not None:
            commit = args.mark_reviewed or args.upstream
            resolved = run_git("rev-parse", commit)
            state["lastReviewedUpstreamCommit"] = resolved
            save_state(state)
            print(f"確認済みコミットを {resolved[:7]} に記録した。")
            return 0

        report_commits(state, args.upstream)
        report_stale_branches(args.upstream)
        report_open_pull_requests(args.upstream)
    except subprocess.CalledProcessError as error:
        message = (error.stderr or "").strip() or error.stdout.strip()
        print(f"上流確認に失敗した: {message}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
