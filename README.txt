SLEEPYSOURCE™ 1.0
=================
Made by SleepyKev • 2026

SleepySource is a Windows streaming tools hub for Kick and OBS. Version 1.0 is the revamped pure C#/.NET build.

ARCHITECTURE
------------
- C# / .NET 8 WinForms desktop application
- Microsoft WebView2 interface
- In-process ASP.NET Core local server
- No Go compatibility engine
- No SleepySource.Engine.exe
- Portable SleepySource_Data beside the application

FEATURES
--------
- Stream Dashboard
- Alert Studio and alert queue
- Kick chat overlay and webhook handling
- Kick connection/authentication tools
- Now Playing / Windows media sessions
- Countdown Pro
- Overlay Designer
- Stream title and category controls
- Profiles, imports/exports and backups
- Cloudflare relay controls
- System Health and diagnostics

OBS URLS
--------
Alerts:      http://127.0.0.1:17891/alerts
Chat:        http://127.0.0.1:17891/chat
Now Playing: http://127.0.0.1:17891/overlay
Countdown:   http://127.0.0.1:17891/countdown
Designer:    http://127.0.0.1:17891/designer

BUILD
-----
Requirements:
- Windows 10/11 x64
- .NET 8 SDK

From the repository root:
  dotnet restore CSharpHost/SleepySource.csproj -r win-x64
  dotnet publish CSharpHost/SleepySource.csproj -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true

The published application is SleepySource.exe. Microsoft Edge WebView2 Runtime is required and is normally included with current Windows 10/11 installations.

DATA COMPATIBILITY
------------------
SleepySource keeps the existing SleepySource_Data layout, local OBS/API address, routes and user-facing workflow from the migration baseline. Do not run another SleepySource build on port 127.0.0.1:17891 at the same time.

VALIDATION
----------
The pure C# Windows build is compiled and runtime-smoke-tested with GitHub Actions. The smoke test launches the real application headlessly, probes the UI/OBS pages and core JSON routes, exercises chat, alerts and countdown controls, verifies backup export, and confirms no legacy Go engine is present in the published output.

WINDOWS
-------
Windows x64 only. This build is unsigned, so Windows SmartScreen may warn on first launch.
