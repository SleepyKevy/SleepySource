#!/usr/bin/env python3
"""Build SleepySource for Windows and embed public-release EXE metadata.

The Go linker gets a .rsrc placeholder from rsrc_windows_amd64.syso. After the
Windows binary is linked, this script rewrites that section with a standards-
compliant RT_ICON / RT_GROUP_ICON resource tree using assets/app.ico plus an
RT_VERSION block so Windows Explorer can display SleepySource product/version
information in the file Properties dialog.
"""
from __future__ import annotations

import os
import re
import struct
import subprocess
import shutil
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent
ICO = ROOT / "assets" / "app.ico"
VERSION_GO = ROOT / "app_types.go"
version_match = re.search(r'appVersion\s*=\s*"([^"]+)"', VERSION_GO.read_text(encoding="utf-8"))
if not version_match:
    raise RuntimeError("could not read appVersion from app_types.go")
VERSION = version_match.group(1)
OUT = ROOT / "SleepySource.exe"



FORBIDDEN_RUNTIME_MARKERS = [
    b"powershell.exe",
    b"ExecutionPolicy",
    b"SetProcessInformation",
    b"SetPriorityClass",
    b"/api/check-update",
    b"update_manifest_url",
    b"performance_guard_enabled",
]


def validate_source_release():
    version_text = VERSION_GO.read_text(encoding="utf-8")
    if f'appVersion            = "{VERSION}"' not in version_text:
        raise RuntimeError(f"release build must use appVersion {VERSION}")

    index = (ROOT / "web" / "index.html").read_text(encoding="utf-8")
    enter = (ROOT / "web" / "enter.html").read_text(encoding="utf-8")
    chat_html = (ROOT / "web" / "chat.html").read_text(encoding="utf-8")
    overlay_html = (ROOT / "web" / "overlay.html").read_text(encoding="utf-8")
    countdown_html = (ROOT / "web" / "countdown.html").read_text(encoding="utf-8")
    alerts_html = (ROOT / "web" / "alerts.html").read_text(encoding="utf-8")
    readme = (ROOT / "README.txt").read_text(encoding="utf-8")
    public_text = "\n".join([index, enter, chat_html, overlay_html, countdown_html, alerts_html, readme])
    retired = ["Press Enter to continue", "chatCopyWebhookBtn", ">BETA<"]
    found = [marker for marker in retired if marker in index]
    if found:
        raise RuntimeError(f"retired UI markers remain in release: {found}")
    for typo in [
        "SleepySouurce", "SleepySource v1.0", "Semi Bold", "Kick Developer app", "Work in Progress",
        "Update Checker Preview Build", "1.1 PREVIEW ADDITION",
        "remaning", "seperate", "recieve", "occured", "succesfully", "definately", "alot",
        "lenght", "widht", "heigth", "choosen", "avaliable", "unavaliable", "compatability",
        "inital", "intial", "proccess", "seperator", "transparant", "adress", "dependancy",
    ]:
        if typo in public_text:
            raise RuntimeError(f"release contains retired/misspelled public text {typo!r}")
    for marker in [
        "OBS Setup Guide",
        "Advanced Diagnostics",
        "chatPrimaryStatus",
        "grid-template-columns:repeat(2,minmax(0,1fr))",
        "grid-template-columns:repeat(3,minmax(0,1fr))",
        ".row>*,.row3>*,.toolbarGrid>*,.chatTestRow>*,.chatStatusGrid>*,.countdownStatusGrid>*{min-width:0}",
        "input,select,button{font:inherit;min-width:0;max-width:100%}",
        "input[type=range]{display:block;width:100%;min-width:0;margin:0}",
        "@media(max-width:760px)",
        "@media(max-width:520px)",
        "clamp(Number(safeStorageGet(sidebarWidthKey,'400')||400),340,650)",
        'id="toolsNavGroup"',
        'id="toolsNavToggle"',
        'aria-controls="moduleNav"',
        'id="moduleNav"',
        'sleepysource-tools-nav-open-v1',
        'function setToolsNavOpen(open,persist=true)',
        '.toolsNavGroup.collapsed .moduleNav{max-height:0;opacity:0;',
        'id="sidebarToolsScroll"',
        'class="sidebarPinnedBottom"',
        '.moduleButton.active{color:#fff;',
        '#nowPlayingSidebar,#chatOverlaySidebar,#countdownSidebar{display:grid;gap:8px;align-content:start;min-width:0}',
        '.accordion{padding:0;overflow:hidden;border-radius:11px;margin:0;min-width:0}',
        '.accordion>summary{list-style:none;display:flex;align-items:center;justify-content:space-between;gap:10px;min-height:38px;padding:8px 10px;',
        "Array.from(panel.children).forEach(other=>{if(other!==section&&other.matches('details.accordion'))other.open=false})",
        "SleepySource readability pass",
        "label{color:#d7e3f0;font-size:13px",
        "input::placeholder{color:#8398b1",
    ]:
        if marker not in index:
            raise RuntimeError(f"release Designer sizing/wording guard is missing {marker!r}")
    ids = re.findall(r'\sid="([^"]+)"', index)
    duplicates = sorted({value for value in ids if ids.count(value) > 1})
    if duplicates:
        raise RuntimeError(f"release Designer contains duplicate HTML ids: {duplicates}")
    if "Work in Progress" in enter:
        raise RuntimeError("release splash still contains pre-release Work in Progress branding")
    if f">Version {VERSION}<" in enter:
        raise RuntimeError(f"release splash still contains the visible Version {VERSION} label")
    for marker in [f"SleepySource {VERSION}", "Open SleepySource", "Ready", f"Build {VERSION}", "particleField"]:
        if marker not in enter:
            raise RuntimeError(f"release splash is missing {marker!r}")

    cloudflare = (ROOT / "cloudflare.go").read_text(encoding="utf-8")
    for marker in [
        'bundledCloudflaredVersion = "2026.7.3"',
        'bundledCloudflaredSHA256  = "8635da433b6df8194746e88ed9d2589566c20e38bfc2a80e431a348b7c765841"',
    ]:
        if marker not in cloudflare:
            raise RuntimeError("release cloudflared pin is missing or unexpected")

    for marker in [f"SLEEPYSOURCE {VERSION}", "WHAT'S NEW IN SLEEPYSOURCE", "QUICK START", "CHAT OVERLAY / KICK SETUP", "ALERT STUDIO", "CONNECTIONS", "COUNTDOWN PRO", "UPGRADING FROM SLEEPYSOURCE", "SOURCE BUILD", "http://127.0.0.1:17891/countdown"]:
        if marker not in readme:
            raise RuntimeError(f"release README is missing {marker!r}")

    countdown_go = (ROOT / "countdown.go").read_text(encoding="utf-8")
    for marker in ["CountdownManager", 'json:"reset_on_unload"', 'case "overlay_loaded"', 'case "overlay_unloaded"']:
        if marker not in countdown_go:
            raise RuntimeError(f"Countdown Pro backend is missing {marker!r}")
    for marker in ["/api/countdown/state", "overlay_loaded", "overlay_unloaded", "s.loop&&d>0"]:
        if marker not in countdown_html:
            raise RuntimeError(f"Countdown Pro Browser Source is missing {marker!r}")
    for marker in ['data-module="countdown-pro"', 'data-countdown-key="mode"', 'id="countdownWorkspace"']:
        if marker not in index:
            raise RuntimeError(f"Countdown Pro Designer is missing {marker!r}")
    alert_go = (ROOT / "alerts.go").read_text(encoding="utf-8")
    for marker in ['AlertManager', 'channel.followed', 'kicks.gifted', 'channel.reward.redemption.updated', 'DisplayMode', 'MediaAboveText', 'TitleFontFamily', 'ExitAnimation']:
        if marker not in alert_go:
            raise RuntimeError(f"Alert Studio backend is missing {marker!r}")
    for marker in ['/api/alerts/state?consumer=overlay', 'class="alert"', 'sound_url', 'sleepysource-alert-layout', 'data-layer="media"']:
        if marker not in alerts_html:
            raise RuntimeError(f"Alert Studio Browser Source is missing {marker!r}")
    for marker in ['data-module="alert-studio"', 'id="alertStudioWorkspace"', 'id="alertStudioSidebar"', 'http://127.0.0.1:17891/alerts', 'id="alertModeAdvanced"', 'id="alertPreviewSelectedBtn"', 'data-alert-key="display_mode"', 'data-alert-key="title_font_family"']:
        if marker not in index:
            raise RuntimeError(f"Alert Studio Designer is missing {marker!r}")
    for marker in ['id="staticHelpButton"', 'class="staticHelpButton"', 'id="helpSidebar"', 'id="helpWorkspace"', 'id="helpNowPlaying"', 'id="helpChat"', 'id="helpCountdown"', 'id="helpOBS"', 'id="helpTroubleshooting"', 'function openHelpGuide()']:
        if marker not in index:
            raise RuntimeError(f"built-in Help & Guide is missing {marker!r}")
    for marker in ['id="moduleToolsLabel">', 'id="homeGlanceTitle">At a glance', 'id="homeGlanceKickValue"', 'id="homeGlanceRelayValue"', 'id="homeGlanceNowValue"', 'class="homeQuickBar homeQuickCompact homeQuickStandalone"', 'class="homeStatusItem" type="button"', 'id="homeUpdateSectionTitle">Updates', 'id="helpAboutTitle">About SleepySource', 'id="headerUpdateIndicator"', 'id="homeUpdateGitHubBtn" hidden>Get Update on GitHub', 'id="helpUpdateGitHubBtn" hidden>Get Update on GitHub', 'id="updateViewReleaseBtn" type="button">Get Update on GitHub']:
        if marker not in index:
            raise RuntimeError(f"SleepySource {VERSION} Home/update UI is missing {marker!r}")
    update_go = (ROOT / "update.go").read_text(encoding="utf-8")
    for marker in ['updateRepositoryURL = "https://github.com/SleepyKevy/SleepySource"', 'go openBrowser(updateRepositoryURL)']:
        if marker not in update_go:
            raise RuntimeError(f"SleepySource GitHub update destination is missing {marker!r}")
    for marker in ['id="staticSystemHealthButton"', '<span class="staticHelpArrow" aria-hidden="true">→</span>', 'id="systemHealthWorkspace"', 'id="healthRunMainBtn"', 'id="healthCopyMainBtn"', "fetch('/api/system/health?ts='+Date.now()", 'function repairHealthAction(action,button)']:
        if marker not in index:
            raise RuntimeError(f"SleepySource System Health UI is missing {marker!r}")
    for marker in ['id="globalSaveBar"', 'id="globalSaveText"', 'id="newProfileBtn"', 'id="duplicateProfileBtn"', 'id="renameProfileBtn"', 'id="defaultProfileBtn"', 'sleepysource-ui-state-v1', 'sleepysource-update-cache-v1', 'Updates are checked only when you choose Check for Updates.', 'id="homeCopyObsSetupBtn"', 'SleepySource OBS Setup']:
        if marker not in index:
            raise RuntimeError(f"SleepySource {VERSION} quality-of-life UI is missing {marker!r}")
    for retired_home_marker in ['id="homeGlanceCountdownValue"', 'id="homeGlanceProfileValue"', 'id="homeGlanceVersion"', 'id="homeResumeBtn"', 'id="homeLastToolValue"', 'id="homeThemeValue"', 'homeContinuePanel', 'homeContinueBody']:
        if retired_home_marker in index:
            raise RuntimeError(f"trimmed Home dashboard unexpectedly contains {retired_home_marker!r}")
    for retired_home_marker in ['homePrimarySection', 'homeModuleGridPrimary', 'data-home-card=', 'homeNowCardStatus', 'homeKickCardStatus', 'homeAlertCardStatus', 'homeStreamCardStatus', 'homeConnectionsCardStatus', 'homeCountdownCardStatus']:
        if retired_home_marker in index:
            raise RuntimeError(f'Home tool-card grid unexpectedly remains: {retired_home_marker!r}')
    if 'checkForUpdates(false)' in index:
        raise RuntimeError(f"SleepySource {VERSION} must not check for updates automatically")
    startup_tail = index[index.rfind('async function poll()'):]
    if 'poll();runSystemHealth' in startup_tail or 'runSystemHealth();setInterval' in startup_tail:
        raise RuntimeError(f"SleepySource {VERSION} must not run System Health automatically at startup")
    if 'data-module="help-guide"' in index:
        raise RuntimeError("Help & Guide should be permanent in the sidebar, not inside module navigation")
    if 'id="moduleSwitcher"' in index or 'class="moduleSwitcher"' in index:
        raise RuntimeError("legacy module dropdown remains; permanent module buttons are required")
    for marker in ['data-module-label="Now Playing"', 'data-module-label="Chat Overlay"', 'data-module-label="Alert Studio"', 'data-module-label="Stream Dashboard"', 'data-module-label="Countdown Pro"', 'id="streamSettingsWorkspace" data-workspace-panel="stream-settings"', 'src="/assets/stream-settings-icon.png"']:
        if marker not in index:
            raise RuntimeError(f"permanent module navigation is missing {marker!r}")
    if 'id="homeStreamMenu"' in index:
        raise RuntimeError("Stream Dashboard must be a main navigation module, not a Home dropdown")
    for marker in ['id="streamSettingsSidebar"', 'data-section="stream-settings-status"', '<summary>Kick Stream Dashboard</summary>']:
        if marker in index:
            raise RuntimeError(f"Stream Dashboard must open directly to its workspace without a sidebar dropdown: {marker!r}")
    for marker in ['Stream tools, all in one place.', 'One click to open', 'class="homeHero homeHeroCompact"']:
        if marker in index:
            raise RuntimeError(f"removed Home hero/resume UI returned: {marker!r}")
    if '<section class="homeSection homeQuickSection">' not in index or '.homeQuickSection{margin-top:0}' not in index:
        raise RuntimeError("Home must start directly with Quick actions after removing the duplicate tool-card grid")
    for marker in ['id="streamWorkspaceAuth"', 'id="streamAuthorizeBtn"', 'id="streamDisconnectAuthBtn"', 'id="streamCopyRedirectBtn"', 'http://127.0.0.1:17891/oauth/kick/callback', "fetch('/api/stream/auth/status'", "fetch('/api/stream/auth/start'", "fetch('/api/stream/auth/disconnect'"]:
        if marker not in index:
            raise RuntimeError(f"Stream Dashboard user authorization UI is missing {marker!r}")
    for marker in ['data-module-label="Connections"', 'id="connectionsWorkspace" data-workspace-panel="connections"', 'id="connectionsAccountTitle"', 'id="connectionsStreamAuthTitle"', 'id="connectionsWebhookTitle"', 'id="connectionsAuthPillCard"']:
        if marker not in index:
            raise RuntimeError(f"Connections control center UI is missing {marker!r}")
    account_pos = index.index('id="connectionsAccountTitle"')
    auth_pos = index.index('id="connectionsStreamAuthTitle"')
    webhook_pos = index.index('id="connectionsWebhookTitle"')
    if not (account_pos < auth_pos < webhook_pos):
        raise RuntimeError("Connections page must keep Kick Account first, Title & Category authorization second, and Chat & Webhook third")
    kick_user_auth = (ROOT / "kick_user_auth.go").read_text(encoding="utf-8")
    for marker in ['kickUserOAuthRedirectURI = "http://127.0.0.1:17891/oauth/kick/callback"', 'kickUserOAuthScopes      = "user:read channel:read channel:write"', 'code_challenge_method', 'protectCredential(plain)', 'handleKickUserAuthCallback']:
        if marker not in kick_user_auth:
            raise RuntimeError(f"Stream Dashboard OAuth implementation is missing {marker!r}")
    stream_settings = (ROOT / "stream_settings.go").read_text(encoding="utf-8")
    for marker in ['fetchKickAuthorizedChannelMetadata(userToken)', 'authorized_channel_matches', 'sameKickBroadcaster(state.BroadcasterUserID, connectedMeta, authorizedMeta)', 'Kick stream settings updated and verified']:
        if marker not in stream_settings:
            raise RuntimeError(f"Stream Dashboard verified-account/update guard is missing {marker!r}")
    if 'a.streamAuth.ensureUserAccessToken(clientID, clientSecret)' not in stream_settings:
        raise RuntimeError("Stream Dashboard updates must use the authorized Kick user access token")
    backup_go = (ROOT / "backup.go").read_text(encoding="utf-8")
    if 'kick_user_authorization.json' not in backup_go or '.kick-user-auth-' not in backup_go:
        raise RuntimeError("Stream Controls credentials must stay excluded from normal backups")
    if 'id="staticSystemHealthButton"' not in index or index.index('id="staticSystemHealthButton"') > index.index('id="staticHelpButton"'):
        raise RuntimeError("System Health must be a standalone pinned bar directly above Help & Guide")
    if 'id="systemHealthSidebar"' in index or 'healthRunSideBtn' in index or 'healthCopySideBtn' in index:
        raise RuntimeError("System Health sidebar controls must stay inside the System Health page")

    gofmt = subprocess.run(["gofmt", "-l", *[str(p) for p in ROOT.glob("*.go")]], cwd=ROOT, capture_output=True, text=True, check=True)
    if gofmt.stdout.strip():
        raise RuntimeError("Go files need gofmt: " + gofmt.stdout.strip().replace("\n", ", "))

    node = shutil.which("node")
    if node:
        import re as _re
        for html in [ROOT / "web" / "index.html", ROOT / "web" / "chat.html", ROOT / "web" / "overlay.html", ROOT / "web" / "countdown.html", ROOT / "web" / "alerts.html", ROOT / "web" / "enter.html"]:
            text = html.read_text(encoding="utf-8")
            scripts = _re.findall(r"<script[^>]*>(.*?)</script>", text, _re.S | _re.I)
            for i, script in enumerate(scripts):
                with tempfile.NamedTemporaryFile("w", suffix=".js", encoding="utf-8", delete=False) as f:
                    f.write(script)
                    temp_name = f.name
                try:
                    subprocess.run([node, "--check", temp_name], check=True, capture_output=True, text=True)
                finally:
                    Path(temp_name).unlink(missing_ok=True)
        print("Validated embedded JavaScript syntax with Node.js.")
    else:
        print("Node.js not found; skipped optional JavaScript syntax validation.")


def validate_pe_section_bounds(exe: Path):
    data = exe.read_bytes()
    if data[:2] != b"MZ":
        raise RuntimeError("output is not a PE executable")
    pe = struct.unpack_from("<I", data, 0x3C)[0]
    if data[pe:pe + 4] != b"PE\0\0":
        raise RuntimeError("output has no PE signature")
    file_header = pe + 4
    section_count = struct.unpack_from("<H", data, file_header + 2)[0]
    optional_size = struct.unpack_from("<H", data, file_header + 16)[0]
    section_table = file_header + 20 + optional_size
    for i in range(section_count):
        sh = section_table + i * 40
        name = bytes(data[sh:sh + 8]).split(b"\0", 1)[0].decode("ascii", errors="replace")
        raw_size, raw_ptr = struct.unpack_from("<II", data, sh + 16)
        if raw_size and (raw_ptr <= 0 or raw_ptr + raw_size > len(data)):
            raise RuntimeError(f"PE section {name!r} extends beyond file bounds")
    print(f"Validated PE section bounds across {section_count} sections ({len(data)} bytes).")


def validate_security_clean_build(exe: Path):
    data = exe.read_bytes()
    found = [m.decode("ascii", errors="replace") for m in FORBIDDEN_RUNTIME_MARKERS if m in data]
    if found:
        raise RuntimeError(f"security-clean build contains forbidden runtime markers: {found}")
    if (ROOT / "media.ps1").exists():
        raise RuntimeError("security-clean build must not contain media.ps1")
    print("Validated security-clean runtime: no PowerShell detector, OBS process tuning, or retired update-check markers.")

def parse_ico(path: Path):
    data = path.read_bytes()
    reserved, typ, count = struct.unpack_from('<HHH', data, 0)
    if reserved != 0 or typ != 1 or count < 1:
        raise RuntimeError('assets/app.ico is not a valid Windows icon file')
    images = []
    for i in range(count):
        off = 6 + i * 16
        (bw, bh, colors, _reserved, planes, bits, size, image_off) = struct.unpack_from('<BBBBHHII', data, off)
        blob = data[image_off:image_off + size]
        if len(blob) != size:
            raise RuntimeError('truncated icon image')
        images.append({
            'bw': bw, 'bh': bh, 'colors': colors, 'planes': planes,
            'bits': bits, 'size': size, 'blob': blob,
        })
    return images


def align4(n: int) -> int:
    return (n + 3) & ~3


def _pad4(blob: bytes) -> bytes:
    return blob + b'\0' * ((4 - (len(blob) % 4)) % 4)


def _version_node(key: str, value: bytes = b'', value_length: int = 0, value_type: int = 0, children=()) -> bytes:
    """Build one VS_VERSIONINFO-family node with DWORD-aligned fields."""
    key_blob = (key + '\0').encode('utf-16le')
    body = bytearray(struct.pack('<HHH', 0, value_length, value_type))
    body += key_blob
    body[:] = _pad4(bytes(body))
    body += value
    body[:] = _pad4(bytes(body))
    for child in children:
        body += child
        body[:] = _pad4(bytes(body))
    if len(body) > 0xFFFF:
        raise RuntimeError(f'version-resource node {key!r} is too large')
    struct.pack_into('<H', body, 0, len(body))
    return bytes(body)


def build_version_info(version: str) -> bytes:
    parts = [int(p) for p in re.findall(r'\d+', version)[:4]]
    parts += [0] * (4 - len(parts))
    major, minor, patch, build = parts
    version_ms = (major << 16) | minor
    version_ls = (patch << 16) | build

    fixed = struct.pack(
        '<13I',
        0xFEEF04BD,  # VS_FFI_SIGNATURE
        0x00010000,  # VS_FFI_STRUCVERSION
        version_ms,
        version_ls,
        version_ms,
        version_ls,
        0x0000003F,  # VS_FFI_FILEFLAGSMASK
        0,
        0x00040004,  # VOS_NT_WINDOWS32
        0x00000001,  # VFT_APP
        0,
        0,
        0,
    )

    strings = {
        'CompanyName': 'SleepyKev',
        'FileDescription': 'SleepySource',
        'FileVersion': f'{major}.{minor}.{patch}.{build}',
        'InternalName': 'SleepySource',
        'LegalCopyright': 'Copyright © 2026 SleepyKev',
        'OriginalFilename': 'SleepySource.exe',
        'ProductName': 'SleepySource',
        'ProductVersion': version,
    }
    string_nodes = []
    for key, text in strings.items():
        encoded = (text + '\0').encode('utf-16le')
        string_nodes.append(_version_node(key, encoded, len(text) + 1, 1))
    string_table = _version_node('040904B0', children=string_nodes)
    string_file_info = _version_node('StringFileInfo', children=[string_table])

    translation = struct.pack('<HH', 0x0409, 1200)
    translation_node = _version_node('Translation', translation, len(translation), 0)
    var_file_info = _version_node('VarFileInfo', children=[translation_node])
    return _version_node('VS_VERSION_INFO', fixed, len(fixed), 0, [string_file_info, var_file_info])


def build_resource(images, section_rva: int) -> bytes:
    """Build a valid resource section with root -> type -> name -> language -> data."""
    RT_ICON = 3
    RT_GROUP_ICON = 14
    RT_VERSION = 16
    LANG_EN_US = 1033
    DIR = 16
    ENTRY = 8
    DATA_ENTRY = 16

    off = 0
    root_off = off
    off += DIR + 3 * ENTRY

    icon_type_off = off
    off += DIR + len(images) * ENTRY
    group_type_off = off
    off += DIR + 1 * ENTRY
    version_type_off = off
    off += DIR + 1 * ENTRY

    icon_lang_dirs = []
    icon_data_entries = []
    for _ in images:
        lang_dir = off
        off += DIR + 1 * ENTRY
        data_entry = off
        off += DATA_ENTRY
        icon_lang_dirs.append(lang_dir)
        icon_data_entries.append(data_entry)

    group_lang_dir = off
    off += DIR + 1 * ENTRY
    group_data_entry = off
    off += DATA_ENTRY

    version_lang_dir = off
    off += DIR + 1 * ENTRY
    version_data_entry = off
    off += DATA_ENTRY

    off = align4(off)
    icon_blob_offsets = []
    for image in images:
        icon_blob_offsets.append(off)
        off = align4(off + image['size'])

    group_blob_offset = off
    group = bytearray(struct.pack('<HHH', 0, 1, len(images)))
    for icon_id, image in enumerate(images, 1):
        group += struct.pack(
            '<BBBBHHIH',
            image['bw'], image['bh'], image['colors'], 0,
            image['planes'], image['bits'], image['size'], icon_id,
        )
    off = align4(off + len(group))

    version_blob = build_version_info(VERSION)
    version_blob_offset = off
    off = align4(off + len(version_blob))

    buf = bytearray(off)

    def put(at, blob):
        buf[at:at + len(blob)] = blob

    def directory(id_count):
        return struct.pack('<IIHHHH', 0, 0, 0, 0, 0, id_count)

    def dir_entry(resource_id, target, is_directory):
        return struct.pack('<II', resource_id, target | (0x80000000 if is_directory else 0))

    def data_entry(data_off, size):
        # OffsetToData is an RVA, not an offset relative to the resource section.
        return struct.pack('<IIII', section_rva + data_off, size, 0, 0)

    # Root directory: RT_ICON, RT_GROUP_ICON, and RT_VERSION.
    put(root_off, directory(3))
    put(root_off + 16, dir_entry(RT_ICON, icon_type_off, True))
    put(root_off + 24, dir_entry(RT_GROUP_ICON, group_type_off, True))
    put(root_off + 32, dir_entry(RT_VERSION, version_type_off, True))

    # RT_ICON -> icon ID -> language -> IMAGE_RESOURCE_DATA_ENTRY.
    put(icon_type_off, directory(len(images)))
    eoff = icon_type_off + 16
    for icon_id, lang_dir in enumerate(icon_lang_dirs, 1):
        put(eoff, dir_entry(icon_id, lang_dir, True))
        eoff += 8

    for i, lang_dir in enumerate(icon_lang_dirs):
        put(lang_dir, directory(1))
        put(lang_dir + 16, dir_entry(LANG_EN_US, icon_data_entries[i], False))
        put(icon_data_entries[i], data_entry(icon_blob_offsets[i], images[i]['size']))

    # RT_GROUP_ICON -> group ID 1 -> language -> group data.
    put(group_type_off, directory(1))
    put(group_type_off + 16, dir_entry(1, group_lang_dir, True))
    put(group_lang_dir, directory(1))
    put(group_lang_dir + 16, dir_entry(LANG_EN_US, group_data_entry, False))
    put(group_data_entry, data_entry(group_blob_offset, len(group)))

    # RT_VERSION -> version ID 1 -> language -> VS_VERSION_INFO.
    put(version_type_off, directory(1))
    put(version_type_off + 16, dir_entry(1, version_lang_dir, True))
    put(version_lang_dir, directory(1))
    put(version_lang_dir + 16, dir_entry(LANG_EN_US, version_data_entry, False))
    put(version_data_entry, data_entry(version_blob_offset, len(version_blob)))

    for image, blob_off in zip(images, icon_blob_offsets):
        put(blob_off, image['blob'])
    put(group_blob_offset, group)
    put(version_blob_offset, version_blob)
    return bytes(buf)


def patch_pe_resource(exe: Path, ico: Path):
    data = bytearray(exe.read_bytes())
    if data[:2] != b'MZ':
        raise RuntimeError('output is not a PE executable')
    pe = struct.unpack_from('<I', data, 0x3C)[0]
    if data[pe:pe+4] != b'PE\0\0':
        raise RuntimeError('output has no PE signature')

    file_header = pe + 4
    machine, section_count, _, _, _, optional_size, _ = struct.unpack_from('<HHIIIHH', data, file_header)
    if machine != 0x8664:
        raise RuntimeError('expected an x64 Windows executable')
    optional = file_header + 20
    magic = struct.unpack_from('<H', data, optional)[0]
    if magic != 0x20B:
        raise RuntimeError('expected PE32+ executable')

    section_table = optional + optional_size
    rsrc_header = None
    for i in range(section_count):
        sh = section_table + i * 40
        name = bytes(data[sh:sh+8]).split(b'\0', 1)[0]
        if name == b'.rsrc':
            rsrc_header = sh
            break
    if rsrc_header is None:
        raise RuntimeError('linked EXE has no .rsrc section to patch')

    virtual_size, virtual_address, raw_size, raw_ptr = struct.unpack_from('<IIII', data, rsrc_header + 8)
    resource = build_resource(parse_ico(ico), virtual_address)
    if len(resource) > raw_size:
        # Go's resource placeholder is normally large enough for the icon data,
        # but the public VERSIONINFO metadata can cross the final alignment
        # boundary. .rsrc is the final section in this release build, so extend
        # it in a PE-compliant way rather than dropping metadata or icon sizes.
        if raw_ptr + raw_size != len(data):
            raise RuntimeError(f'resource ({len(resource)} bytes) exceeds reserved .rsrc size ({raw_size} bytes) and .rsrc is not the final section')
        section_alignment = struct.unpack_from('<I', data, optional + 32)[0]
        file_alignment = struct.unpack_from('<I', data, optional + 36)[0]
        new_raw_size = (len(resource) + file_alignment - 1) // file_alignment * file_alignment
        data.extend(b'\0' * (new_raw_size - raw_size))
        raw_size = new_raw_size
        struct.pack_into('<I', data, rsrc_header + 16, raw_size)
        size_of_image = (virtual_address + len(resource) + section_alignment - 1) // section_alignment * section_alignment
        struct.pack_into('<I', data, optional + 56, size_of_image)

    data[raw_ptr:raw_ptr + raw_size] = resource + b'\0' * (raw_size - len(resource))
    struct.pack_into('<I', data, rsrc_header + 8, len(resource))  # VirtualSize

    # PE32+ data directories start at optional-header offset 112. Entry 2 is resources.
    resource_directory = optional + 112 + 2 * 8
    struct.pack_into('<II', data, resource_directory, virtual_address, len(resource))
    exe.write_bytes(data)


def validate_resource(exe: Path):
    """Small independent check that Explorer-relevant resource types are present."""
    data = exe.read_bytes()
    pe = struct.unpack_from('<I', data, 0x3C)[0]
    fh = pe + 4
    sections = struct.unpack_from('<H', data, fh + 2)[0]
    opt_size = struct.unpack_from('<H', data, fh + 16)[0]
    opt = fh + 20
    dd = opt + 112
    resource_rva, resource_size = struct.unpack_from('<II', data, dd + 16)
    st = opt + opt_size
    raw = None
    for i in range(sections):
        sh = st + i * 40
        vs, va, rs, rp = struct.unpack_from('<IIII', data, sh + 8)
        if va <= resource_rva < va + max(vs, rs):
            raw = rp + (resource_rva - va)
            break
    if raw is None or resource_size < 32:
        raise RuntimeError('resource directory was not written correctly')
    named, ids = struct.unpack_from('<HH', data, raw + 12)
    types = []
    for i in range(named + ids):
        rid, target = struct.unpack_from('<II', data, raw + 16 + i * 8)
        if rid & 0x80000000 == 0:
            types.append(rid)
    if 3 not in types or 14 not in types or 16 not in types:
        raise RuntimeError(f'public EXE resource validation failed; resource types={types}')
    for marker in ['SleepySource', VERSION, 'SleepyKev']:
        if marker.encode('utf-16le') not in data:
            raise RuntimeError(f'VERSIONINFO validation failed; missing {marker!r}')
    print(f'Validated EXE resources: RT_ICON, RT_GROUP_ICON, and RT_VERSION present ({resource_size} bytes).')


def main():
    validate_source_release()
    env = os.environ.copy()
    env.update({'GOOS': 'windows', 'GOARCH': 'amd64', 'GOAMD64': 'v1', 'CGO_ENABLED': '0'})
    # Keep the release script self-checking: tests and vet must pass before a
    # Windows executable is produced. GOAMD64=v1 maximizes x64 CPU compatibility.
    subprocess.run(['go', 'test', './...'], cwd=ROOT, check=True)
    subprocess.run(['go', 'vet', './...'], cwd=ROOT, check=True)
    subprocess.run([
        'go', 'build', '-trimpath', '-buildvcs=false', '-ldflags=-H=windowsgui -s -w', '-o', str(OUT), '.'
    ], cwd=ROOT, env=env, check=True)
    validate_pe_section_bounds(OUT)
    validate_security_clean_build(OUT)
    patch_pe_resource(OUT, ICO)
    validate_resource(OUT)
    validate_pe_section_bounds(OUT)
    validate_security_clean_build(OUT)
    print(f'Built: {OUT}')


if __name__ == '__main__':
    main()
