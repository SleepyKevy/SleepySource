SLEEPYSOURCE™ 1.0.0
=======================================
Made by SleepyKev • 2026

WHAT SLEEPYSOURCE IS
--------------------
SleepySource is a Windows streaming companion for Kick + OBS. It combines a
local desktop dashboard, OBS Browser Source overlays, hosted Kick OAuth/event
delivery, stream controls, profile/backup tools, and system diagnostics in one
portable application.

CORE FEATURES
-------------
- Home command-center dashboard with stream/tool status at a glance.
- Standalone Connections tab for the shared Kick account connection.
- Hosted “Connect with Kick” OAuth flow; users do not enter Client ID, Client
 Secret, access tokens, refresh tokens, webhook URLs, or Cloudflare settings.
- Stream Dashboard for reading/updating supported Kick stream metadata.
- Now Playing overlay with media detection and visual customization.
- Chat Overlay with Kick events, badges/emotes, themes, and layout controls.
- Alert Studio with follow, subscription, gifted subscription, KICKs/gifts,
 and reward-redemption alert support.
- Countdown Pro with configurable timer, styling, profiles, and OBS output.
- Overlay/profile backup and restore support.
- System Health checks for the local server, storage, OBS routes, hosted API,
 Kick authorization, event subscriptions, realtime delivery, and fallback.

NAVIGATION
----------
Home Overview, status, OBS links, updates, and quick actions.
Connections Connect/reconnect/disconnect Kick and inspect event status.
Tools Now Playing, Chat Overlay, Alert Studio, Stream Dashboard,
 and Countdown Pro.
System Health Diagnostics and repair actions.
Help & Guide Built-in setup and usage reference.

QUICK START
-----------
1. Extract the Public ZIP to a normal writable folder.
2. Run SleepySource.exe.
3. Open Connections and choose Connect with Kick.
4. Complete the browser authorization flow.
5. Return to SleepySource and confirm Account / Kick Events / Realtime status.
6. Add the desired OBS Browser Source URL(s) below.
7. Match each Browser Source width/height to the canvas configured in its tool.

OBS BROWSER SOURCE URLS
-----------------------
Now Playing: http://127.0.0.1:17891/overlay
Chat: http://127.0.0.1:17891/chat
Alerts: http://127.0.0.1:17891/alerts
Countdown: http://127.0.0.1:17891/countdown
Designer: http://127.0.0.1:17891/designer

LOCAL DATA
----------
SleepySource keeps portable settings beside the application in:
 SleepySource_Data\

This folder contains local tool settings, profiles, uploaded media references,
and the encrypted SleepySource connection credential used by the desktop app.
Keep the folder with the executable when moving a configured installation.

KICK / SECURITY MODEL
---------------------
- Kick OAuth access and refresh tokens remain on the hosted SleepySource service.
- The desktop stores only an opaque SleepySource connection ID/token pair.
- The saved desktop connection credential is protected with Windows credential
 protection before being written locally.
- Kick Client Secret and token-encryption secrets belong only on Cloudflare.
- Realtime events use an authenticated WebSocket fast path.
- A D1-backed polling/acknowledgement queue provides reliable fallback delivery.

HOSTED KICK EVENTS
------------------
- chat.message.sent
- channel.followed
- channel.subscription.new
- channel.subscription.renewal
- channel.subscription.gifts
- kicks.gifted
- channel.reward.redemption.updated

SYSTEM REQUIREMENTS
-------------------
- Windows 10 or Windows 11, x64.
- Microsoft Edge WebView2 Runtime.
- Internet access for hosted Kick authentication/events and update checks.
- OBS Studio only if using Browser Source overlays.

The Public build is self-contained for .NET. The Source build requires the
.NET 8 SDK to compile.

SOURCE BUILD
------------
From the Source package root on Windows:
 powershell -ExecutionPolicy Bypass -File .\BUILD_RELEASE.ps1

Equivalent manual commands:
 dotnet restore CSharpHost\SleepySource.csproj -r win-x64
 dotnet publish CSharpHost\SleepySource.csproj -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true --no-restore -o "dist\SleepySource 1.0.0"

The source also includes a GitHub Actions Windows runtime-smoke workflow.

CLOUDFLARE BACKEND
------------------
Matching backend source is included in:
 CloudflareWorker\worker.js

Deployment details are in:
 CloudflareWorker\DEPLOYMENT.md

The backend requires the documented Kick OAuth secrets, D1 binding, and
Durable Object binding. Never place server secrets in the desktop source.

UPDATES
-------
SleepySource checks the configured SleepySource GitHub release only when the user
chooses Check for Updates. The current update service targets:
 SleepyKevy/SleepySource-2.0

IMPORTANT NOTES
---------------
- SleepySource currently listens locally on 127.0.0.1:17891.
- The Windows executable is unsigned; SmartScreen may warn on first launch.
- Do not delete SleepySource_Data if you want to keep local configuration.
- Read release_notes.txt for the complete consolidated 1.0.0 changelog and
 pre-release development history.

KICK EVENT REPAIR
If Kick is connected but chat/alerts stop arriving, open Connections and click
Sync Kick Events. SleepySource rebuilds its managed Kick subscriptions against
the current hosted webhook URL without disconnecting your Kick account.
