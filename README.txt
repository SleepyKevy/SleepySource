[See the changelog](https://github.com/SleepyKevy/SleepySource-2.0/blob/main/RELEASE_NOTES.txt)

SLEEPYSOURCE™ 1.0
=================
Made by SleepyKev • 2026

SleepySource 1.0 is a Windows streaming tools hub for Kick and OBS. This revamp is implemented in C#/.NET 8 while preserving the existing SleepySource interface, OBS Browser Source workflow and portable data layout.

ARCHITECTURE
------------
- C# / .NET 8 WinForms desktop application
- Microsoft WebView2 interface
- In-process ASP.NET Core local HTTP/OBS server
- C# services for alerts, chat, countdown, Kick, profiles/backups, Cloudflare relay, Windows media sessions, health and updates
- No Go compatibility engine
- No SleepySource.Engine.exe
- SleepySource_Data remains portable beside the application

FEATURES
--------
- Stream Dashboard
- Alert Studio and alert queue
- Kick chat overlay and integration
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

COMPATIBILITY
-------------
The revamp preserves the local service address at 127.0.0.1:17891, existing OBS routes, SleepySource_Data layout, settings/profile formats and user-facing workflow from the migration baseline.

VALIDATION
----------
The C# rewrite is compiled and runtime-tested on Windows through GitHub Actions. A frozen pre-removal Go baseline was used during migration to compare route surface, default JSON contracts, mutations, backup restore and restart persistence. The final release tree contains no Go source or Go runtime dependency.

WINDOWS
-------
Windows x64 only. This build is unsigned, so Windows SmartScreen may warn on first launch.
