#!/usr/bin/env python3
"""Build an independent copy of the Windows ChatGPT desktop with Codex multiplexing.

The official app (an MSIX package) is read as build input only and never modified.
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

# Reviewed official builds. Both the version and the app.asar SHA-256 must match;
# anything else fails closed.
TESTED_SOURCE_BUILDS = {
    "26.818.8289.0": "e2f04d6aa921d07981b42368df0a28a8bebe8cd21375d4a1f9286757b51c1313",
}

# Minified identifiers from the official build that the injected UI references.
# They change between builds, so every declaration is verified before substitution.
RENDERER_BINDINGS = {
    # placeholder: (identifier, a fragment that uniquely marks its declaration)
    "__CODEX_MUX_JSX__": ("u7", "u7=J()"),
    "__CODEX_MUX_REACT__": ("nql", "nql=r(s(),1)"),
    "__CODEX_MUX_MENU_ITEM__": ("fI", "function fI("),
    "__CODEX_MUX_MENU__": ("vI", "vI={Trigger:"),
    "__CODEX_MUX_IMAGE_URL__": ("Ija", "function Ija("),
    "__CODEX_MUX_INTL__": ("pd", "function pd()"),
}

# Message ids for the injected UI. They do not exist in the official locale bundles,
# so translations are inserted for the languages that have them. Everything else falls
# back to defaultMessage, which is English.
MESSAGE_ID_PREFIX = "codexMux."
TRANSLATION_ROOT = PROJECT_ROOT / "ui" / "messages"

# The depletion alerts use official message ids, and those ids already have official
# translations. formatjs prefers a translation over defaultMessage, so the id has to be
# replaced with ours or the injected wording never appears.
DEPLETION_MESSAGE_ID = f"{MESSAGE_ID_PREFIX}allSubscriptionsDepleted"
DEPLETION_DEFAULT_MESSAGE = "All connected subscriptions are depleted"
DEPLETION_ALERTS = (
    ("codex.rateLimitResetPurchaseModal.title", "You’re out of usage"),
    ("codex.upsellBanner.merged.title", "You’re out of Codex and Work usage"),
    (
        "codex.upsellBanner.merged.workspaceUsage.title",
        "You’ve used all Codex and Work usage",
    ),
    (
        "codex.upsellBanner.merged.legacy.headline.noReset",
        "You’ve reached your usage limit",
    ),
)

# Native modules that must stay unpacked rather than being absorbed into the ASAR.
ASAR_UNPACK_DIRECTORIES = "node_modules/{@worklouder,better-sqlite3,node-pty}"
REQUIRED_UNPACKED_MODULE = (
    "unpack : /node_modules/better-sqlite3/build/Release/better_sqlite3.node"
)


def lists_unpacked_module(listing: str) -> bool:
    """Normalize separators: `asar list --is-pack` uses the platform separator."""
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
        help="The official app directory. Detected from the MSIX package when omitted.",
    )
    parser.add_argument("--destination", type=Path, default=DEFAULT_DESTINATION)
    parser.add_argument(
        "--force",
        action="store_true",
        help="Replace an existing destination after moving it to a recoverable backup.",
    )
    parser.add_argument(
        "--allow-untested-source",
        action="store_true",
        help="Continue after an explicit version or app.asar hash mismatch.",
    )
    return parser.parse_args()


def run(command: list[str], *, cwd: Path | None = None) -> None:
    subprocess.run(command, cwd=cwd, check=True)


def output(command: list[str]) -> str:
    return subprocess.check_output(command, text=True).strip()


def require_tool(name: str) -> None:
    if shutil.which(name) is None:
        raise RuntimeError(f"required tool not found: {name}")


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
    """Quote a value safely as a PowerShell single-quoted string."""
    return "'" + str(value).replace("'", "''") + "'"


def locate_official_app() -> tuple[Path, str]:
    """Locate the official app directory from the installed MSIX package."""
    raw = powershell(
        f"$p = Get-AppxPackage -Name {PACKAGE_NAME} | "
        "Sort-Object Version | Select-Object -Last 1; "
        "if ($null -eq $p) { '' } else { $p.InstallLocation + '|' + $p.Version }"
    )
    if not raw or "|" not in raw:
        raise RuntimeError(
            f"the official ChatGPT desktop (MSIX package {PACKAGE_NAME}) was not found; "
            "install it from the Microsoft Store first"
        )
    install_location, version = raw.split("|", 1)
    app_directory = Path(install_location) / "app"
    if not (app_directory / "resources" / "app.asar").is_file():
        raise RuntimeError(f"not the expected official app layout: {app_directory}")
    return app_directory, version.strip()


def source_version(app_directory: Path) -> str:
    """Read the version from the MSIX AppxManifest. Used when --source is given."""
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
        raise RuntimeError(f"quit the running component before replacing it: {destination}")


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
        raise RuntimeError("run `npm ci --ignore-scripts` before patching")
    actual = json.loads(manifest.read_text(encoding="utf-8")).get("version")
    if actual != expected:
        raise RuntimeError(
            f"installed @electron/asar is {actual!r}, expected {expected!r}; "
            "run `npm ci --ignore-scripts`"
        )
    return asar


def restrict_to_current_user(path: Path) -> None:
    """Break ACL inheritance so only the current user can reach the path.

    Changing an ACL can require SeSecurityPrivilege and fail in some environments.
    Everything under %USERPROFILE% is already limited to that user by default, so a
    failure only warns instead of stopping the install.
    """
    target = quote_for_powershell(path)
    try:
        _restrict_to_current_user(target)
    except subprocess.CalledProcessError:
        print(
            f"Warning: could not harden the ACL on {path}; "
            "it keeps the default %USERPROFILE% permissions.",
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
    # Only set the ACL when creating the root. Rewriting an existing state root would
    # strip the permission the official app adds for its Windows sandbox
    # (CodexSandboxUsers).
    created = not DEFAULT_STATE_ROOT.exists()
    DEFAULT_STATE_ROOT.mkdir(parents=True, exist_ok=True)
    if created:
        restrict_to_current_user(DEFAULT_STATE_ROOT)
    token_path = DEFAULT_STATE_ROOT / "control-token"
    if token_path.exists():
        token = token_path.read_text(encoding="utf-8").strip()
        if re.fullmatch(r"[0-9a-f]{64}", token) is None:
            raise RuntimeError(f"invalid control token at {token_path}")
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
            f"could not uniquely locate {description} ({occurrences} matches); "
            "the official build layout may have changed"
        )
    return text.replace(anchor, replacement, 1)


def bind_renderer_identifiers(component: str, bundle: str) -> str:
    """Bind the injected UI's placeholders to identifiers in the official build.

    The identifiers depend on the minifier, so stop when a declaration cannot be
    confirmed. Injecting an unbound placeholder crashes the renderer at runtime.
    """
    for placeholder, (identifier, declaration) in RENDERER_BINDINGS.items():
        if declaration not in bundle:
            raise RuntimeError(
                f"the declaration ({declaration}) of {identifier}, required by the injected UI, "
                "was not found in the official build; upstream may have changed"
            )
        component = component.replace(placeholder, identifier)

    remaining = re.findall(r"__CODEX_MUX_[A-Z_]+__", component)
    if remaining:
        raise RuntimeError(f"unbound placeholders remain: {sorted(set(remaining))}")
    return component


def single_asset(directory: Path, pattern: str, description: str) -> Path:
    matches = sorted(directory.glob(pattern))
    if len(matches) != 1:
        raise RuntimeError(f"found {len(matches)} matches for {description}, expected exactly one")
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
        "the renderer CSP connect-src",
    )
    index_path.write_text(index, encoding="utf-8")

    bundle_path = single_asset(assets, "app-initial-*.js", "the initial renderer bundle")
    bundle = bundle_path.read_text(encoding="utf-8")
    if "function CodexMuxAccountMenu(" in bundle:
        raise RuntimeError("the source app already contains the multiplexer menu")

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
        "the native profile menu component",
    )

    bundle = replace_once(
        bundle,
        "let e=await F_.safeGet(`/wham/profiles/me`)",
        "let e=await codexMuxProfileData("
        "globalThis.__codexMuxSelectedProfileAccountId??null)",
        "the native profile stats request",
    )

    usage_modal_anchor = "function hsc(e){let t=(0,_sc.c)(28),{defaultResetCreditsOpen:n,"
    bundle = replace_once(
        bundle,
        usage_modal_anchor,
        "function hsc(e){CodexMuxUseResetAccountState();"
        "let t=(0,_sc.c)(28),{defaultResetCreditsOpen:n,",
        "the native usage modal",
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
        "the native reset-credit query",
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
        "the native reset-credit mutation",
    )

    bundle = replace_once(
        bundle,
        "let y=v;if(g!=null){",
        "let y=window.__codexMuxSelectedUsageWindows??v;if(g!=null){",
        "the native usage-window selection",
    )

    bundle = replace_once(
        bundle,
        "usageItems:Ct",
        "usageItems:(0,e7.jsx)(CodexMuxAccountMenu,{})",
        "the native usage menu slot",
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
            "a native profile menu open-state hook",
        )

    for message_id, default_message in DEPLETION_ALERTS:
        bundle = replace_once(
            bundle,
            f"id:`{message_id}`,defaultMessage:`{default_message}`",
            f"id:`{DEPLETION_MESSAGE_ID}`,defaultMessage:`{DEPLETION_DEFAULT_MESSAGE}`",
            "a native subscription depletion alert",
        )
    bundle_path.write_text(bundle, encoding="utf-8")
    patch_profile_page(assets)
    patch_locale_messages(assets)


def patch_locale_messages(assets: Path) -> None:
    """Insert the injected UI's translations into the official locale bundles.

    The official app looks a message id up in each language's dictionary and falls back
    to defaultMessage when it is missing. Adding our ids to the dictionaries we have
    translations for gives the injected UI the same behaviour: translated where a
    translation exists, English everywhere else.
    """
    if not TRANSLATION_ROOT.is_dir():
        raise RuntimeError(f"translation directory not found: {TRANSLATION_ROOT}")

    translated = sorted(TRANSLATION_ROOT.glob("*.json"))
    if not translated:
        raise RuntimeError("no translation files were found")

    for translation_path in translated:
        locale = translation_path.stem
        messages = json.loads(translation_path.read_text(encoding="utf-8"))
        unexpected = [key for key in messages if not key.startswith(MESSAGE_ID_PREFIX)]
        if unexpected:
            raise RuntimeError(
                f"{translation_path.name} contains unexpected message ids: {unexpected[:3]}"
            )
        bundle_path = single_asset(
            assets, f"{locale}-*.js", f"the {locale} locale bundle"
        )
        bundle_path.write_text(
            insert_locale_messages(
                bundle_path.read_text(encoding="utf-8"), messages, locale
            ),
            encoding="utf-8",
        )
    print(f"Injected translations into: {', '.join(path.stem for path in translated)}")


def insert_locale_messages(bundle: str, messages: dict[str, str], locale: str) -> str:
    """Add translations to a locale bundle's message dictionary.

    The dictionary is assigned to the variable published by `export{<var> as default}`.
    Its values are template literals, so write ours as JSON strings to keep ICU braces
    from being read as interpolation.
    """
    exported = re.search(r"export\s*\{\s*([A-Za-z_$][\w$]*)\s+as\s+default\b", bundle)
    if exported is None:
        raise RuntimeError(f"{locale}: could not identify the message dictionary variable")
    variable = exported.group(1)

    anchor = f"{variable}={{"
    if bundle.count(anchor) != 1:
        raise RuntimeError(
            f"{locale}: could not uniquely locate the message dictionary "
            f"({bundle.count(anchor)} matches)"
        )
    if MESSAGE_ID_PREFIX in bundle:
        raise RuntimeError(f"{locale}: the injected UI translations are already present")

    entries = "".join(
        f"{json.dumps(key, ensure_ascii=False)}:"
        f"{json.dumps(value, ensure_ascii=False)},"
        for key, value in messages.items()
    )
    return bundle.replace(anchor, anchor + entries, 1)


def patch_profile_page(assets: Path) -> None:
    """Show that the profile page covers every subscription.

    The statistics are already pooled, but the header shows one account's identity, so
    the page reads as a single account. Only replace it when two or more are connected.
    """
    bundle_path = single_asset(assets, "profile-*.js", "the Profile settings bundle")
    bundle = bundle_path.read_text(encoding="utf-8")

    replacements = (
        (
            'avatar:(0,$.jsxs)($.Fragment,{children:[(0,$.jsxs)(`label`,'
            '{"aria-disabled":I.isPending,',
            "avatar:globalThis.CodexMuxProfileAvatarStack?.()??"
            '(0,$.jsxs)($.Fragment,{children:[(0,$.jsxs)(`label`,'
            '{"aria-disabled":I.isPending,',
            "the Profile avatar",
        ),
        (
            "displayName:et??(0,$.jsx)(J,{id:`profile.nameFallback`",
            "displayName:globalThis.CodexMuxProfileDisplayName?.()??et??"
            "(0,$.jsx)(J,{id:`profile.nameFallback`",
            "the Profile display name",
        ),
        (
            "username:Qe==null?null:(0,$.jsx)(J,{id:`profile.usernameValue`",
            "username:globalThis.CodexMuxProfileUsername?.()??"
            "(Qe==null?null:(0,$.jsx)(J,{id:`profile.usernameValue`",
            "the Profile username",
        ),
    )
    for anchor, replacement, description in replacements:
        bundle = replace_once(bundle, anchor, replacement, description)

    # The username replacement wrapped a conditional, so close the extra parenthesis.
    username_tail = (
        "description:`Profile username shown with an at-sign prefix`,"
        "values:{username:Qe}})"
    )
    bundle = replace_once(
        bundle,
        username_tail,
        username_tail + ")",
        "the Profile username closing parenthesis",
    )
    bundle_path.write_text(bundle, encoding="utf-8")


def patch_desktop_profile(extracted: Path) -> None:
    """Give the copied app its own user data area and the diagnostic bridge."""
    build = extracted / ".vite" / "build"
    bootstrap_path = single_asset(build, "bootstrap-*.js", "the bootstrap bundle")
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
        raise RuntimeError("could not isolate the copied desktop profile")
    bootstrap_path.write_text(bootstrap, encoding="utf-8")

    main_path = single_asset(build, "main-*.js", "the desktop main bundle")
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
        "the Computer Use tool instruction",
    )

    shutil.copy2(PROJECT_ROOT / "ui" / "ui-test-bridge.cjs", build / "ui-test-bridge.cjs")
    main += (
        "\n;if(process.env.CODEX_MUX_UI_TESTS===`1`)"
        "require(require(`node:path`).join(__dirname,`ui-test-bridge.cjs`)).start();"
    )
    main_path.write_text(main, encoding="utf-8")


def patch_owl_configuration(staged_app: Path) -> None:
    """Give the owl shell its own user data name so it cannot collide with the official app."""
    ini_path = staged_app / "resources" / "owl-app.ini"
    if not ini_path.is_file():
        raise RuntimeError("owl-app.ini was not found")
    ini = ini_path.read_text(encoding="utf-8")
    updated, replacements = re.subn(
        r"^UserDataDirectoryName=.*$",
        f"UserDataDirectoryName={DESKTOP_PROFILE_NAME}",
        ini,
        count=1,
        flags=re.MULTILINE,
    )
    if replacements != 1:
        raise RuntimeError("could not rewrite UserDataDirectoryName in owl-app.ini")
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
    # robocopy reports success as 0-7 and failure as 8 or above.
    if result.returncode >= 8:
        raise RuntimeError(f"could not copy the official app (robocopy {result.returncode})")


def resolve_source(source: Path | None) -> tuple[Path, str]:
    if source is None:
        return locate_official_app()
    resolved = source.expanduser().resolve()
    if not (resolved / "resources" / "app.asar").is_file():
        raise RuntimeError(f"not an official app directory: {resolved}")
    return resolved, source_version(resolved)


def verify_source(source: Path, version: str, allow_untested_source: bool) -> None:
    print("Verifying the official app.asar…")
    digest = file_digest(source / "resources" / "app.asar")
    expected = TESTED_SOURCE_BUILDS.get(version)
    print(f"Official ChatGPT version: {version}, app.asar {digest}")
    if expected == digest:
        return
    if not allow_untested_source:
        raise RuntimeError(
            "the official version or app.asar hash is not approved; review the upstream "
            "change or pass --allow-untested-source"
        )
    print(
        "Warning: continuing with an unverified official build; the patch continues "
        "only while every expected anchor matches.",
        file=sys.stderr,
    )


def patch_app(
    source: Path | None,
    destination: Path,
    force: bool,
    allow_untested_source: bool,
) -> None:
    if sys.platform != "win32":
        raise RuntimeError("this patcher is Windows only")

    source, version = resolve_source(source)
    destination = destination.expanduser().resolve()
    if source == destination:
        raise RuntimeError("source and destination must differ; the official app is never patched")
    if destination.exists() and not force:
        raise RuntimeError(
            f"destination exists: {destination} "
            "(pass --force to create a recoverable backup)"
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

        print("Building the multiplexer…")
        build_go_binary("./cmd/codex-mux", proxy)
        print("Building the launcher…")
        build_go_binary("./cmd/launcher", launcher, "-H=windowsgui")

        print("Copying the official app…")
        copy_official_app(source, staged_app)

        resources = staged_app / "resources"
        original_asar = resources / "app.asar"

        print("Patching the desktop profile and renderer…")
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
            raise RuntimeError("native ASAR modules were not kept unpacked")
        shutil.copy2(repacked_asar, original_asar)

        repacked_unpacked = temporary_path / "app.asar.unpacked"
        if not repacked_unpacked.is_dir():
            raise RuntimeError("ASAR pack did not produce its unpacked native tree")
        shutil.copytree(
            repacked_unpacked, resources / "app.asar.unpacked", dirs_exist_ok=True
        )

        bundled_codex = resources / "codex.exe"
        real_codex = resources / "codex.real.exe"
        if real_codex.exists():
            raise RuntimeError("the source app already contains codex.real.exe")
        if not bundled_codex.is_file():
            raise RuntimeError("codex.exe was not found in the official app")
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
                print(f"Existing copy moved to {app_backup}")
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
        print(f"patch failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
