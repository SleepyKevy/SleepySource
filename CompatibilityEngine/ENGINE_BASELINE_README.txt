SLEEPYSOURCE 1.3.2
Official Icon & UI Remake Fix Release
Made by SleepyKev • 2026

SleepySource is a portable Windows x64 streaming utility for OBS. It includes Now Playing, Chat Overlay, Alert Studio, Stream Dashboard, Connections, and Countdown Pro in one native desktop app.

============================================================
WHAT'S NEW IN SLEEPYSOURCE 1.3.2
============================================================

• Sidebar navigation now uses one clean monochrome icon family instead of mixed emoji and image styles.
• Home, Now Playing, Chat Overlay, Alert Studio, Stream Dashboard, Connections, Countdown Pro, Tools, and the System Health tab now share consistent sizing and visual weight.
• Tools uses a dedicated monochrome wrench icon, and the System Health tab uses a dedicated monochrome health/diagnostic icon.
• System Health page status/warning icons remain unchanged.
• Home and Stream Dashboard navigation artwork was cleaned up to match the new icon system.
• Mascot/theme image edges were softened for cleaner rendering at smaller UI sizes.
• The global save indicator is smaller and clearer, and several tiny secondary labels were increased for readability.
• Home sidebar duplication was reduced so the main System Status area remains the primary status view.
• Existing Alert Studio, Kick, relay, Stream Dashboard, Countdown Pro, OBS Browser Sources, settings, profiles, and saved data behavior remain intact.
• Update checks and System Health checks remain manual and user-controlled.

============================================================
QUICK START
============================================================

1. Extract the full SleepySource 1.3.2 folder to a normal writable location. Do not run it from inside the ZIP.
2. Run SleepySource.exe.
3. Select Open SleepySource on the splash screen.
4. Configure the tools you want to use.
5. In OBS, add Browser Sources using the local URLs below and match their configured canvas sizes.
6. Keep SleepySource running or hidden in the Windows system tray while OBS uses its Browser Sources.

OBS BROWSER SOURCE URLS

Now Playing   http://127.0.0.1:17891/overlay
Chat Overlay  http://127.0.0.1:17891/chat
Alert Studio  http://127.0.0.1:17891/alerts
Countdown Pro http://127.0.0.1:17891/countdown

Home > OBS URLs also provides individual Copy actions and Copy OBS Setup.

============================================================
CORE TOOLS
============================================================

NOW PLAYING
• Native Windows media-session detection.
• Editable artwork, text, progress, time, effects, animation, and layout controls.
• Profiles, custom fonts, designer tools, import/export, defaults, and backup support.

CHAT OVERLAY / KICK SETUP
• Connect your Kick Developer App from Connections.
• Signed Kick webhooks are displayed in the local /chat Browser Source.
• Supports Kick/7TV emotes, badges, avatars, custom fonts, themes, styling, animations, and test messages.
• Cloudflare Quick Tunnel relay URLs are temporary and can change after restarting the relay.

ALERT STUDIO
• One transparent /alerts Browser Source with queued playback.
• Supports follows, new subscriptions, renewals, gifted subscriptions, Kicks/gifts, and rewards.
• Each alert type keeps independent media, sound, text, typography, geometry, placement, timing, and enter/exit animation settings.
• Includes designer preview, live OBS tests, editable test data, skip/clear queue controls, Copy Style, and Reset Selected Alert.

STREAM DASHBOARD
• Load and update the connected Kick channel title and category.
• Stream Dashboard authorization uses Kick user OAuth with PKCE and channel read/write permissions.
• Authorization is managed from Connections and can be disconnected without removing Chat Overlay credentials.

CONNECTIONS
• Kick account connection.
• Stream Dashboard title/category authorization.
• Relay, webhook, subscription, and connection status in one place.

COUNTDOWN PRO
• Countdown and stopwatch modes.
• Presets, profiles, custom formats, loop/overtime behavior, fonts, effects, panel styling, motion, and OBS load/unload behavior.

============================================================
SYSTEM HEALTH, SAVING, AND UPDATES
============================================================

System Health is a diagnostic workspace and does not alter OBS output. Run Health Check manually to inspect the local server, data storage, custom media, profiles, OBS routes, Kick/webhook state, relay state, and end-to-end connectivity.

SleepySource remembers supported settings and interface state and shows a global Save Status for supported designer tools.

Update checks are manual. SleepySource does not automatically download, install, or replace SleepySource.exe. Official releases are published at:
https://github.com/SleepyKevy/SleepySource

============================================================
UPGRADING FROM SLEEPYSOURCE 1.3.x
============================================================

1. Exit SleepySource completely from the system tray.
2. Make a full backup or copy your existing SleepySource_Data folder somewhere safe.
3. Extract SleepySource 1.3.2 to a writable folder.
4. To keep portable data, place the existing SleepySource_Data folder beside the new SleepySource.exe, or replace only the old EXE after making a backup.
5. Start 1.3.2 and confirm profiles, media, fonts, Kick settings, Alert Studio settings, Countdown settings, and OBS canvas sizes.
6. If you use a Cloudflare Quick Tunnel, confirm the current public webhook URL after upgrading or restarting the relay.

============================================================
DATA, BACKUPS, SECURITY, AND PRIVACY
============================================================

• User data is stored beside the EXE in SleepySource_Data for portable use.
• Keep a separate backup before replacing an existing install.
• Saved secrets use Windows-protected credential storage where supported.
• Stream Dashboard authorization credentials are excluded from normal backup bundles.
• The local app server binds to 127.0.0.1:17891 rather than a public network interface.
• The Cloudflare relay is used only when you start it for external Kick webhook delivery.
• Never share client secrets, access tokens, refresh tokens, or unreviewed diagnostic/private backup data.

============================================================
REQUIREMENTS
============================================================

• 64-bit Windows
• Microsoft Edge WebView2 Runtime
• OBS Studio for Browser Sources
• Internet access for Kick, 7TV, GitHub update checks, and Cloudflare relay features when used

============================================================
TROUBLESHOOTING
============================================================

APP DOES NOT OPEN
• Extract the ZIP first and use a writable folder.
• Confirm Microsoft Edge WebView2 Runtime is installed.

OBS SOURCE IS BLANK
• Confirm SleepySource is running.
• Re-copy the correct local URL from Home > OBS URLs.
• Match the Browser Source canvas size to SleepySource and refresh the OBS Browser Source.

KICK CONNECTS BUT CHAT OR ALERTS ARE MISSING
• Open Connections and verify Kick, relay, webhook, and subscription status.
• Restart the relay if needed and confirm the current temporary webhook URL.
• Re-register subscriptions when the relay URL changes.
• Run System Health for focused diagnostics.

ALERT DOES NOT LOOK OR MOVE CORRECTLY
• Test the selected alert from Alert Studio before testing in OBS.
• Confirm the selected alert type has the intended media, timing, placement, enter animation, and exit animation.
• Refresh the /alerts Browser Source after major layout changes.

============================================================
SOURCE BUILD
============================================================

Requirements: Go and Python 3.

From the source folder:

  go test ./...
  go vet ./...
  python build_windows.py

The release builder validates source guards, Go tests, go vet, a Windows x64 build, PE section bounds, Windows icon/version resources, and release-safety markers before accepting SleepySource.exe.

============================================================
RELEASE
============================================================

SleepySource 1.3.2
Made by SleepyKev • 2026
