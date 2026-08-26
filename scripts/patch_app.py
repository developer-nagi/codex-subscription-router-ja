#!/usr/bin/env python3
"""Windows 版 ChatGPT デスクトップの独立コピーを作り、Codex 多重化を注入する。

公式アプリ (MSIX パッケージ) は入力としてのみ読み取り、決して変更しない。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parent.parent
PROJECT_VERSION = (PROJECT_ROOT / "VERSION").read_text(encoding="utf-8").strip()

PACKAGE_NAME = "OpenAI.Codex"
APP_DISPLAY_NAME = "Codex Subscription Router"
DESKTOP_PROFILE_NAME = "Codex Subscription Router"
LAUNCHER_NAME = "CodexSubscriptionRouter.exe"
CONTROL_PORT = 48123

DEFAULT_DESTINATION = (
    Path(os.environ.get("LOCALAPPDATA", Path.home() / "AppData" / "Local"))
    / "Programs"
    / APP_DISPLAY_NAME
)
DEFAULT_STATE_ROOT = Path.home() / ".codex-mux"

# 検証済みの公式ビルド。バージョンと app.asar SHA-256 の両方で fail-closed に判定する。
TESTED_SOURCE_BUILDS = {
    "26.818.8289.0": "e2f04d6aa921d07981b42368df0a28a8bebe8cd21375d4a1f9286757b51c1313",
}

# 注入する UI が参照する、公式ビルド側の minify 済み識別子。
# 値はビルドごとに変わるため、置換前に宣言の存在を必ず検証する。
RENDERER_BINDINGS = {
    # プレースホルダ: (識別子, その識別子の宣言を一意に示す断片)
    "__CODEX_MUX_JSX__": ("u7", "u7=J()"),
    "__CODEX_MUX_REACT__": ("nql", "nql=r(s(),1)"),
    "__CODEX_MUX_MENU_ITEM__": ("fI", "function fI("),
    "__CODEX_MUX_MENU__": ("vI", "vI={Trigger:"),
    "__CODEX_MUX_IMAGE_URL__": ("Ija", "function Ija("),
}

# ASAR 内に取り込まず展開したまま残す必要があるネイティブモジュール。
ASAR_UNPACK_DIRECTORIES = "node_modules/{@worklouder,better-sqlite3,node-pty}"
REQUIRED_UNPACKED_MODULE = (
    "unpack : /node_modules/better-sqlite3/build/Release/better_sqlite3.node"
)


def lists_unpacked_module(listing: str) -> bool:
    """`asar list --is-pack` の区切り文字は OS 依存なので正規化して判定する。"""
    return REQUIRED_UNPACKED_MODULE in listing.replace("\\", "/")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--version", action="version", version=f"%(prog)s {PROJECT_VERSION}"
    )
    parser.add_argument(
        "--source",
        type=Path,
        default=None,
        help="公式アプリの app ディレクトリ。省略時は MSIX パッケージを自動検出する。",
    )
    parser.add_argument("--destination", type=Path, default=DEFAULT_DESTINATION)
    parser.add_argument(
        "--force",
        action="store_true",
        help="既存のインストール先を復元可能なバックアップへ退避してから置き換える。",
    )
    parser.add_argument(
        "--allow-untested-source",
        action="store_true",
        help="バージョンまたは app.asar ハッシュの不一致を明示的に許容する。",
    )
    return parser.parse_args()


def run(command: list[str], *, cwd: Path | None = None) -> None:
    subprocess.run(command, cwd=cwd, check=True)


def output(command: list[str]) -> str:
    return subprocess.check_output(command, text=True).strip()


def require_tool(name: str) -> None:
    if shutil.which(name) is None:
        raise RuntimeError(f"必要なツールが見つからない: {name}")


def powershell(script: str) -> str:
    return subprocess.check_output(
        [
            "powershell.exe",
            "-NoProfile",
            "-NonInteractive",
            "-ExecutionPolicy",
            "Bypass",
            "-Command",
            script,
        ],
        text=True,
    ).strip()


def quote_for_powershell(value: object) -> str:
    """PowerShell の単一引用符文字列として安全に埋め込む。"""
    return "'" + str(value).replace("'", "''") + "'"


def locate_official_app() -> tuple[Path, str]:
    """インストール済み MSIX から公式アプリの app ディレクトリを特定する。"""
    raw = powershell(
        f"$p = Get-AppxPackage -Name {PACKAGE_NAME} | "
        "Sort-Object Version | Select-Object -Last 1; "
        "if ($null -eq $p) { '' } else { $p.InstallLocation + '|' + $p.Version }"
    )
    if not raw or "|" not in raw:
        raise RuntimeError(
            f"公式 ChatGPT デスクトップ (MSIX パッケージ {PACKAGE_NAME}) が見つからない。"
            "先に Microsoft Store から導入する"
        )
    install_location, version = raw.split("|", 1)
    app_directory = Path(install_location) / "app"
    if not (app_directory / "resources" / "app.asar").is_file():
        raise RuntimeError(f"想定した公式アプリ構成ではない: {app_directory}")
    return app_directory, version.strip()


def source_version(app_directory: Path) -> str:
    """MSIX の AppxManifest からバージョンを読む。--source 指定時に使う。"""
    manifest = app_directory.parent / "AppxManifest.xml"
    if manifest.is_file():
        text = manifest.read_text(encoding="utf-8", errors="replace")
        match = re.search(r'<Identity[^>]*\sVersion="([\d.]+)"', text)
        if match is not None:
            return match.group(1)
    return "unknown"


def file_digest(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def ensure_components_are_stopped(destination: Path) -> None:
    marker = quote_for_powershell(str(destination) + "*")
    running = powershell(
        "@(Get-CimInstance Win32_Process | Where-Object { "
        f"$_.ExecutablePath -like {marker} }}).Count"
    )
    if running.isdigit() and int(running) > 0:
        raise RuntimeError(f"起動中のコンポーネントを終了してから再実行する: {destination}")


def ensure_asar_tool() -> Path:
    binaries = PROJECT_ROOT / "node_modules" / ".bin"
    asar = binaries / "asar.cmd"
    if not asar.exists():
        asar = binaries / "asar"
    manifest = PROJECT_ROOT / "node_modules" / "@electron" / "asar" / "package.json"
    expected = json.loads((PROJECT_ROOT / "package.json").read_text(encoding="utf-8"))[
        "devDependencies"
    ]["@electron/asar"]
    if not asar.exists() or not manifest.is_file():
        raise RuntimeError("先に `npm ci --ignore-scripts` を実行する")
    actual = json.loads(manifest.read_text(encoding="utf-8")).get("version")
    if actual != expected:
        raise RuntimeError(
            f"@electron/asar のバージョンが {actual!r} で期待値 {expected!r} と異なる。"
            "`npm ci --ignore-scripts` を実行する"
        )
    return asar


def restrict_to_current_user(path: Path) -> None:
    """継承 ACL を切り、現在のユーザーだけがアクセスできる状態にする。

    ACL の変更は環境によっては特権 (SeSecurityPrivilege) を要求され失敗する。
    `%USERPROFILE%` 配下は既定で当該ユーザーのみに制限されているため、失敗しても
    導入自体は続行し、強化できなかったことを警告する。
    """
    target = quote_for_powershell(path)
    try:
        _restrict_to_current_user(target)
    except subprocess.CalledProcessError:
        print(
            f"警告: {path} の ACL を強化できなかった。"
            "%USERPROFILE% 既定の権限のままとなる。",
            file=sys.stderr,
        )


def _restrict_to_current_user(target: str) -> None:
    powershell(
        f"$path = {target}; "
        "$acl = Get-Acl -LiteralPath $path; "
        "$acl.SetAccessRuleProtection($true, $false); "
        "foreach ($rule in @($acl.Access)) { [void]$acl.RemoveAccessRule($rule) }; "
        "$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().User; "
        "$inheritance = [System.Security.AccessControl.InheritanceFlags]::None; "
        "if ((Get-Item -LiteralPath $path) -is [System.IO.DirectoryInfo]) { "
        "$inheritance = [System.Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit' }; "
        "$rule = New-Object System.Security.AccessControl.FileSystemAccessRule("
        "$identity, "
        "[System.Security.AccessControl.FileSystemRights]::FullControl, "
        "$inheritance, "
        "[System.Security.AccessControl.PropagationFlags]::None, "
        "[System.Security.AccessControl.AccessControlType]::Allow); "
        "$acl.AddAccessRule($rule); "
        "Set-Acl -LiteralPath $path -AclObject $acl"
    )


def load_or_create_token() -> str:
    # ACL は新規作成時にだけ設定する。既存の状態ルートを書き換えると、公式アプリが
    # Windows サンドボックス用に追加する権限 (CodexSandboxUsers) を消してしまう。
    created = not DEFAULT_STATE_ROOT.exists()
    DEFAULT_STATE_ROOT.mkdir(parents=True, exist_ok=True)
    if created:
        restrict_to_current_user(DEFAULT_STATE_ROOT)
    token_path = DEFAULT_STATE_ROOT / "control-token"
    if token_path.exists():
        token = token_path.read_text(encoding="utf-8").strip()
        if re.fullmatch(r"[0-9a-f]{64}", token) is None:
            raise RuntimeError(f"制御トークンが不正: {token_path}")
        return token
    token = secrets.token_hex(32)
    descriptor = os.open(token_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
        handle.write(token)
    restrict_to_current_user(token_path)
    return token


def build_go_binary(package: str, destination: Path, extra_ldflags: str = "") -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    ldflags = "-s -w" + (" " + extra_ldflags if extra_ldflags else "")
    run(
        [
            "go",
            "build",
            "-trimpath",
            f"-ldflags={ldflags}",
            "-o",
            str(destination),
            package,
        ],
        cwd=PROJECT_ROOT,
    )


def replace_once(text: str, anchor: str, replacement: str, description: str) -> str:
    occurrences = text.count(anchor)
    if occurrences != 1:
        raise RuntimeError(
            f"{description}を一意に特定できない (一致 {occurrences} 件)。"
            "公式ビルドの構成が変わった可能性がある"
        )
    return text.replace(anchor, replacement, 1)


def bind_renderer_identifiers(component: str, bundle: str) -> str:
    """注入する UI のプレースホルダを、公式ビルド側の識別子へ結び付ける。

    識別子は minify 結果に依存するため、宣言の存在を確認できない場合は停止する。
    未束縛のプレースホルダを残したまま注入すると、レンダラーが実行時に落ちる。
    """
    for placeholder, (identifier, declaration) in RENDERER_BINDINGS.items():
        if declaration not in bundle:
            raise RuntimeError(
                f"注入 UI が必要とする識別子 {identifier} の宣言 ({declaration}) が"
                "公式ビルドに見つからない。上流の構成が変わった可能性がある"
            )
        component = component.replace(placeholder, identifier)

    remaining = re.findall(r"__CODEX_MUX_[A-Z_]+__", component)
    if remaining:
        raise RuntimeError(f"未束縛のプレースホルダが残っている: {sorted(set(remaining))}")
    return component


def single_asset(directory: Path, pattern: str, description: str) -> Path:
    matches = sorted(directory.glob(pattern))
    if len(matches) != 1:
        raise RuntimeError(f"{description}が {len(matches)} 件見つかった (1 件であるべき)")
    return matches[0]


def patch_renderer(extracted: Path, token: str) -> None:
    webview = extracted / "webview"
    assets = webview / "assets"

    index_path = webview / "index.html"
    index = index_path.read_text(encoding="utf-8")
    connect_anchor = "connect-src &#39;self&#39;"
    index = replace_once(
        index,
        connect_anchor,
        f"{connect_anchor} http://127.0.0.1:{CONTROL_PORT}",
        "レンダラー CSP の connect-src",
    )
    index_path.write_text(index, encoding="utf-8")

    bundle_path = single_asset(assets, "app-initial-*.js", "初期レンダラーバンドル")
    bundle = bundle_path.read_text(encoding="utf-8")
    if "function CodexMuxAccountMenu(" in bundle:
        raise RuntimeError("入力アプリに既に多重化メニューが含まれている")

    component = (PROJECT_ROOT / "ui" / "account-menu.js").read_text(encoding="utf-8")
    component = component.replace("__CODEX_MUX_CONTROL_PORT__", str(CONTROL_PORT))
    component = component.replace("__CODEX_MUX_CONTROL_TOKEN__", token)
    component = bind_renderer_identifiers(component, bundle)

    component_anchor = (
        "function XKl(e){let t=(0,tql.c)(253),{sidebarFooter:n,triggerButton:r}=e"
    )
    bundle = replace_once(
        bundle,
        component_anchor,
        component + "\n" + component_anchor,
        "ネイティブのプロフィールメニューコンポーネント",
    )

    bundle = replace_once(
        bundle,
        "let e=await F_.safeGet(`/wham/profiles/me`)",
        "let e=await codexMuxProfileData("
        "globalThis.__codexMuxSelectedProfileAccountId??null)",
        "ネイティブのプロフィール統計リクエスト",
    )

    usage_modal_anchor = "function hsc(e){let t=(0,_sc.c)(28),{defaultResetCreditsOpen:n,"
    bundle = replace_once(
        bundle,
        usage_modal_anchor,
        "function hsc(e){CodexMuxUseResetAccountState();"
        "let t=(0,_sc.c)(28),{defaultResetCreditsOpen:n,",
        "ネイティブの使用量モーダル",
    )

    reset_query_anchor = (
        "function TCa(){let e=(0,kV.c)(1),t;return "
        "e[0]===Symbol.for(`react.memo_cache_sentinel`)?"
        "(t={queryKey:[`rate-limit-reset-credits`],queryFn:ECa,"
        "refetchInterval:Dp.ONE_MINUTE,staleTime:Dp.FIVE_SECONDS},e[0]=t):"
        "t=e[0],It(t)}"
    )
    bundle = replace_once(
        bundle,
        reset_query_anchor,
        "function TCa(){let e=window.__codexMuxResetAccountId;return It({"
        "queryKey:[`rate-limit-reset-credits`,e??`primary`],"
        "queryFn:e?()=>codexMuxRateLimitResets(e):ECa,"
        "refetchInterval:Dp.ONE_MINUTE,staleTime:Dp.FIVE_SECONDS})}",
        "ネイティブのリセットクレジット取得クエリ",
    )

    reset_mutation_anchor = (
        "function DCa(){let e=(0,kV.c)(3),t=ct(),n=hb(),r;return "
        "e[0]!==n||e[1]!==t?(r={mutationFn:OCa,onSuccess:(e,r)=>{"
        "let{creditId:i}=r,a=e.code;if(a===`reset`||a===`already_redeemed`){"
        "let n=e.code===`reset`?e.credit?.id??i:i;"
        "t.setQueryData([`rate-limit-reset-credits`],e=>ZSa(e,a,n))}"
        "Promise.all([n([`rate-limit-status`]),n([`rate-limit-reset-credits`])])}},"
        "e[0]=n,e[1]=t,e[2]=r):r=e[2],Qt(r)}"
    )
    bundle = replace_once(
        bundle,
        reset_mutation_anchor,
        "function DCa(){let e=ct(),t=hb(),n=window.__codexMuxResetAccountId,"
        "r=[`rate-limit-reset-credits`,n??`primary`];return Qt({"
        "mutationFn:n?i=>codexMuxConsumeRateLimitReset(n,i):OCa,"
        "onSuccess:(n,i)=>{let{creditId:a}=i,o=n.code;"
        "if(o===`reset`||o===`already_redeemed`){let t=o===`reset`?"
        "n.credit?.id??a:a;e.setQueryData(r,e=>ZSa(e,o,t))}"
        "Promise.all([t([`rate-limit-status`]),t(r)])}})}",
        "ネイティブのリセットクレジット消費ミューテーション",
    )

    bundle = replace_once(
        bundle,
        "let y=v;if(g!=null){",
        "let y=window.__codexMuxSelectedUsageWindows??v;if(g!=null){",
        "ネイティブの使用量ウィンドウ選択",
    )

    bundle = replace_once(
        bundle,
        "usageItems:Ct",
        "usageItems:(0,e7.jsx)(CodexMuxAccountMenu,{})",
        "ネイティブの使用量メニュースロット",
    )

    open_change_anchors = (
        ("open:s,onOpenChange:l,contentWidth:`panel`,triggerButton:Dt", "onOpenChange:l", "l"),
        ("open:u,onOpenChange:d,align:`start`,contentWidth:`panel`", "onOpenChange:d", "d"),
    )
    for anchor, original, variable in open_change_anchors:
        bundle = replace_once(
            bundle,
            anchor,
            anchor.replace(
                original, f"onOpenChange:CodexMuxProfileMenuOpenChange({variable})"
            ),
            "ネイティブのプロフィールメニュー開閉フック",
        )

    depleted_message = "接続中のすべてのサブスクリプションの利用枠を使い切りました"
    depleted_alert_anchors = (
        "defaultMessage:`You’re out of Codex and Work usage`",
        "defaultMessage:`You’ve used all Codex and Work usage`",
        "defaultMessage:`You’ve reached your usage limit`",
        "defaultMessage:`You’re out of usage`",
    )
    for anchor in depleted_alert_anchors:
        bundle = replace_once(
            bundle,
            anchor,
            f"defaultMessage:`{depleted_message}`",
            "ネイティブの利用枠切れアラート",
        )
    bundle_path.write_text(bundle, encoding="utf-8")
    patch_profile_page(assets)


def patch_profile_page(assets: Path) -> None:
    """プロフィール設定が合算であることを示す。

    統計は全サブスクリプションの合算だが、ヘッダーは単一アカウントの識別情報を
    出すため合算だと分からない。接続が 2 件以上のときだけ差し替える。
    """
    bundle_path = single_asset(assets, "profile-*.js", "プロフィール設定バンドル")
    bundle = bundle_path.read_text(encoding="utf-8")

    replacements = (
        (
            'avatar:(0,$.jsxs)($.Fragment,{children:[(0,$.jsxs)(`label`,'
            '{"aria-disabled":I.isPending,',
            "avatar:globalThis.CodexMuxProfileAvatarStack?.()??"
            '(0,$.jsxs)($.Fragment,{children:[(0,$.jsxs)(`label`,'
            '{"aria-disabled":I.isPending,',
            "プロフィールのアバター",
        ),
        (
            "displayName:et??(0,$.jsx)(J,{id:`profile.nameFallback`",
            "displayName:globalThis.CodexMuxProfileDisplayName?.()??et??"
            "(0,$.jsx)(J,{id:`profile.nameFallback`",
            "プロフィールの表示名",
        ),
        (
            "username:Qe==null?null:(0,$.jsx)(J,{id:`profile.usernameValue`",
            "username:globalThis.CodexMuxProfileUsername?.()??"
            "(Qe==null?null:(0,$.jsx)(J,{id:`profile.usernameValue`",
            "プロフィールのユーザー名",
        ),
    )
    for anchor, replacement, description in replacements:
        bundle = replace_once(bundle, anchor, replacement, description)

    # username は三項演算子を包み直したため、対応する括弧を閉じる。
    username_tail = (
        "description:`Profile username shown with an at-sign prefix`,"
        "values:{username:Qe}})"
    )
    bundle = replace_once(
        bundle,
        username_tail,
        username_tail + ")",
        "プロフィールのユーザー名の閉じ括弧",
    )
    bundle_path.write_text(bundle, encoding="utf-8")


def patch_desktop_profile(extracted: Path) -> None:
    """コピーしたアプリに専用のユーザーデータ領域と診断ブリッジを与える。"""
    build = extracted / ".vite" / "build"
    bootstrap_path = single_asset(build, "bootstrap-*.js", "ブートストラップバンドル")
    bootstrap = bootstrap_path.read_text(encoding="utf-8")

    profile_pattern = re.compile(
        r"(?P<electron>[A-Za-z_$][\w$]*)\.app\.setPath\("
        r"`userData`,[A-Za-z_$][\w$]*\(\{"
        r"appDataPath:(?P=electron)\.app\.getPath\(`appData`\),"
        r"buildFlavor:[^,}]+,env:process\.env\}\)\)"
    )

    def replacement(match: re.Match[str]) -> str:
        electron = match.group("electron")
        return (
            f"{electron}.app.setPath(`userData`,"
            f"{electron}.app.getPath(`appData`)+`/{DESKTOP_PROFILE_NAME}`)"
        )

    bootstrap, replacements = profile_pattern.subn(replacement, bootstrap, count=1)
    if replacements != 1:
        raise RuntimeError("コピーしたデスクトッププロファイルを分離できない")
    bootstrap_path.write_text(bootstrap, encoding="utf-8")

    main_path = single_asset(build, "main-*.js", "メインプロセスバンドル")
    main = main_path.read_text(encoding="utf-8")
    strict_computer_use_instruction = (
        "Control desktop apps on Windows through Computer Use via node_repl and "
        "@oai/sky only. Never use shell commands, PowerShell, WScript, or synthetic "
        "input APIs for computer interactions or as a fallback. If Computer Use is "
        "unavailable, report the failure instead of using another automation method."
    )
    main = replace_once(
        main,
        "Control desktop apps on macOS through Computer Use.",
        strict_computer_use_instruction,
        "Computer Use のツール指示",
    )

    shutil.copy2(PROJECT_ROOT / "ui" / "ui-test-bridge.cjs", build / "ui-test-bridge.cjs")
    main += (
        "\n;if(process.env.CODEX_MUX_UI_TESTS===`1`)"
        "require(require(`node:path`).join(__dirname,`ui-test-bridge.cjs`)).start();"
    )
    main_path.write_text(main, encoding="utf-8")


def patch_owl_configuration(staged_app: Path) -> None:
    """owl シェルのユーザーデータ名を独立させ、公式アプリと衝突させない。"""
    ini_path = staged_app / "resources" / "owl-app.ini"
    if not ini_path.is_file():
        raise RuntimeError("owl-app.ini が見つからない")
    ini = ini_path.read_text(encoding="utf-8")
    updated, replacements = re.subn(
        r"^UserDataDirectoryName=.*$",
        f"UserDataDirectoryName={DESKTOP_PROFILE_NAME}",
        ini,
        count=1,
        flags=re.MULTILINE,
    )
    if replacements != 1:
        raise RuntimeError("owl-app.ini の UserDataDirectoryName を書き換えられない")
    ini_path.write_text(updated, encoding="utf-8")


def create_start_menu_shortcut(destination: Path) -> Path:
    programs = (
        Path(os.environ["APPDATA"]) / "Microsoft" / "Windows" / "Start Menu" / "Programs"
    )
    programs.mkdir(parents=True, exist_ok=True)
    shortcut = programs / f"{APP_DISPLAY_NAME}.lnk"
    powershell(
        "$shell = New-Object -ComObject WScript.Shell; "
        f"$link = $shell.CreateShortcut({quote_for_powershell(shortcut)}); "
        f"$link.TargetPath = {quote_for_powershell(destination / LAUNCHER_NAME)}; "
        f"$link.WorkingDirectory = {quote_for_powershell(destination)}; "
        f"$link.Description = {quote_for_powershell(APP_DISPLAY_NAME)}; "
        "$link.Save()"
    )
    return shortcut


def copy_official_app(source: Path, staged: Path) -> None:
    result = subprocess.run(
        [
            "robocopy",
            str(source),
            str(staged),
            "/E",
            "/NFL",
            "/NDL",
            "/NJH",
            "/NJS",
            "/NP",
            "/R:1",
            "/W:1",
        ],
        check=False,
    )
    # robocopy は 0-7 が成功、8 以上が失敗を表す。
    if result.returncode >= 8:
        raise RuntimeError(f"公式アプリのコピーに失敗した (robocopy {result.returncode})")


def resolve_source(source: Path | None) -> tuple[Path, str]:
    if source is None:
        return locate_official_app()
    resolved = source.expanduser().resolve()
    if not (resolved / "resources" / "app.asar").is_file():
        raise RuntimeError(f"公式アプリの app ディレクトリではない: {resolved}")
    return resolved, source_version(resolved)


def verify_source(source: Path, version: str, allow_untested_source: bool) -> None:
    print("公式アプリの app.asar を検証中…")
    digest = file_digest(source / "resources" / "app.asar")
    expected = TESTED_SOURCE_BUILDS.get(version)
    print(f"公式 ChatGPT バージョン: {version}, app.asar {digest}")
    if expected == digest:
        return
    if not allow_untested_source:
        raise RuntimeError(
            "公式アプリのバージョンまたは app.asar ハッシュが検証済みの値と一致しない。"
            "上流の変更を確認するか --allow-untested-source を明示的に指定する"
        )
    print(
        "警告: 未検証の公式ビルドで続行する。"
        "期待するアンカーが全て一致する場合のみパッチは継続する。",
        file=sys.stderr,
    )


def patch_app(
    source: Path | None,
    destination: Path,
    force: bool,
    allow_untested_source: bool,
) -> None:
    if sys.platform != "win32":
        raise RuntimeError("このパッチャーは Windows 専用")

    source, version = resolve_source(source)
    destination = destination.expanduser().resolve()
    if source == destination:
        raise RuntimeError("入力と出力は別である必要がある。公式アプリは決して変更しない")
    if destination.exists() and not force:
        raise RuntimeError(
            f"インストール先が既に存在する: {destination} "
            "(--force で復元可能なバックアップを作成して置き換える)"
        )

    verify_source(source, version, allow_untested_source)

    for tool in ("go", "npm", "robocopy", "powershell.exe"):
        require_tool(tool)
    asar = ensure_asar_tool()
    token = load_or_create_token()

    if destination.exists():
        ensure_components_are_stopped(destination)

    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(
        prefix=".codex-subscription-router-", dir=destination.parent
    ) as temporary:
        temporary_path = Path(temporary)
        staged_app = temporary_path / destination.name
        extracted = temporary_path / "asar"
        proxy = temporary_path / "codex-mux.exe"
        launcher = temporary_path / LAUNCHER_NAME

        print("多重化プロキシをビルド中…")
        build_go_binary("./cmd/codex-mux", proxy)
        print("ランチャーをビルド中…")
        build_go_binary("./cmd/launcher", launcher, "-H=windowsgui")

        print("公式アプリをコピー中…")
        copy_official_app(source, staged_app)

        resources = staged_app / "resources"
        original_asar = resources / "app.asar"

        print("デスクトッププロファイルとレンダラーをパッチ中…")
        run([str(asar), "extract", str(original_asar), str(extracted)])
        patch_desktop_profile(extracted)
        patch_renderer(extracted, token)

        repacked_asar = temporary_path / "app.asar"
        run(
            [
                str(asar),
                "pack",
                "--unpack-dir",
                ASAR_UNPACK_DIRECTORIES,
                str(extracted),
                str(repacked_asar),
            ]
        )
        listing = output([str(asar), "list", "--is-pack", str(repacked_asar)])
        if not lists_unpacked_module(listing):
            raise RuntimeError("ネイティブ ASAR モジュールが展開状態で保持されていない")
        shutil.copy2(repacked_asar, original_asar)

        repacked_unpacked = temporary_path / "app.asar.unpacked"
        if not repacked_unpacked.is_dir():
            raise RuntimeError("ASAR の pack がネイティブモジュールを展開しなかった")
        shutil.copytree(
            repacked_unpacked, resources / "app.asar.unpacked", dirs_exist_ok=True
        )

        bundled_codex = resources / "codex.exe"
        real_codex = resources / "codex.real.exe"
        if real_codex.exists():
            raise RuntimeError("入力アプリに既に codex.real.exe が含まれている")
        if not bundled_codex.is_file():
            raise RuntimeError("公式アプリに codex.exe が見つからない")
        bundled_codex.rename(real_codex)
        shutil.copy2(proxy, bundled_codex)

        shutil.copy2(launcher, staged_app / LAUNCHER_NAME)
        patch_owl_configuration(staged_app)

        backup_directory = DEFAULT_STATE_ROOT / "backups" / time.strftime("%Y%m%d-%H%M%S")
        app_backup = backup_directory / destination.name
        had_app = destination.exists()
        if had_app:
            backup_directory.mkdir(parents=True, exist_ok=True)
        try:
            if had_app:
                destination.rename(app_backup)
                print(f"既存のコピーを {app_backup} へ退避した")
            staged_app.rename(destination)
        except OSError:
            if app_backup.exists() and not destination.exists():
                app_backup.rename(destination)
            raise

    shortcut = create_start_menu_shortcut(destination)
    print(destination)
    print(shortcut)


def main() -> int:
    args = parse_args()
    try:
        patch_app(args.source, args.destination, args.force, args.allow_untested_source)
    except (RuntimeError, OSError, subprocess.CalledProcessError) as error:
        print(f"パッチ失敗: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
