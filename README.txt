SLEEPYSOURCE™ 1.0 BETA
=====================
Made by SleepyKev • 2026

This is a completely separate C#/.NET migration beta. It does not replace or modify the existing Go-based SleepySource release project.

WHAT THIS BETA IS
-----------------
- Native C#/.NET 8 WinForms desktop host.
- Microsoft WebView2 desktop UI.
- The SleepySource 1.3.2 interface, overlays and OBS routes are preserved.
- The splash identifies this build as SleepySource™ 1.0 Beta.
- C# owns the visible desktop window, tray behavior and backend process lifecycle.
- A headless SleepySource 1.3.2 compatibility engine is bundled temporarily so existing features continue working during the backend migration.

CURRENT COMPATIBILITY
---------------------
The compatibility engine preserves the existing local service and routes including:
- Home / Stream Dashboard
- Alert Studio and /alerts
- Chat Overlay and /chat
- Now Playing and /overlay
- Countdown Pro and /countdown
- Overlay Designer
- Connections / Kick integration
- Cloudflare relay controls
- Stream title/category controls
- Profiles, imports/exports and backups
- System Health and diagnostics
- Existing SleepySource_Data formats

IMPORTANT
---------
This beta is NOT yet a pure-C# backend rewrite. The desktop application is C#/.NET, while the mature 1.3.2 Go backend is running headlessly as a compatibility engine. This is intentional for the first full migration beta so feature behavior is preserved while services are moved to C# one-by-one.

Do not run the normal SleepySource build and SleepySource 1.0 Beta at the same time. Both use the local OBS/API port 127.0.0.1:17891.

FILES
-----
SleepySource.exe          C#/.NET desktop host
SleepySource.Engine.exe   Temporary headless 1.3.2 compatibility engine
assets/                   Splash/app assets
web/                      C# splash page
SleepySource_Data/        Created automatically on first run

OBS URLS
--------
Alerts:      http://127.0.0.1:17891/alerts
Chat:        http://127.0.0.1:17891/chat
Now Playing: http://127.0.0.1:17891/overlay
Countdown:   http://127.0.0.1:17891/countdown

WINDOWS
-------
Windows x64 only.
Microsoft Edge WebView2 Runtime is required and is normally included with current Windows 10/11 installations.
This beta is unsigned, so Windows SmartScreen may warn on first launch.

BETA FIXES
----------
- Chat Overlay Copy URL buttons are explicitly wired to the WebView2-safe clipboard helper.
- Copy URL / Copy Source actions now include a WebView2-safe clipboard fallback.
- OBS Chat no longer rebuilds and re-animates every existing message when a new message arrives, preventing the full overlay from blinking/flickering on updates.


TRADEMARK
---------
SleepySource™ is used as the visible product/brand name in this Beta. The ™ mark is intentionally limited to branding surfaces and is not being added to installer or application metadata in this build.

Mouse Back/Forward buttons are intentionally disabled inside SleepySource. The desktop host blocks WebView2 Back/Forward navigation and clears browser session history after the dashboard opens.
Desktop behavior update:
- Minimizing SleepySource now keeps its taskbar button visible.
- The system tray icon remains active at the same time while SleepySource is running.
- Closing with the window X now exits SleepySource completely, including the tray icon and compatibility engine.

