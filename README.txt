SLEEPYSOURCE™ 1.1
=================
Made by SleepyKev • 2026

SleepySource 1.1 is a Windows streaming tools hub for Kick and OBS, built as a self-contained C#/.NET 8 application with a WebView2 interface and local OBS Browser Source server.

CORE
----
- C# / .NET 8 WinForms desktop application
- Microsoft WebView2 interface
- In-process ASP.NET Core local HTTP/OBS server
- No Go compatibility engine or SleepySource.Engine.exe
- Portable SleepySource_Data folder beside the application

FEATURES
--------
- Stream Dashboard
- Branded startup splash
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

COMPATIBILITY
-------------
SleepySource 1.1 keeps the existing 127.0.0.1:17891 service address, OBS routes, SleepySource_Data layout, settings/profile formats and normal user workflow.

SOURCE BUILD
------------
Requirements:
- Windows 10/11 x64
- .NET 8 SDK

From the repository root:
  dotnet restore CSharpHost/SleepySource.csproj -r win-x64
  dotnet publish CSharpHost/SleepySource.csproj -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true

The published application is SleepySource.exe. Microsoft Edge WebView2 Runtime is required and is normally included with current Windows 10/11 installations.

WINDOWS
-------
Windows x64 only. SleepySource is currently unsigned, so Windows SmartScreen may warn on first launch.
