$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$project = Join-Path $root 'CSharpHost\SleepySource.csproj'
$out = Join-Path $root 'dist\SleepySource 1.0.0'

if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
    throw '.NET 8 SDK is required. Install the .NET 8 SDK, then run this script again.'
}

$version = & dotnet --version
Write-Host "Using .NET SDK $version"

if (Test-Path $out) { Remove-Item $out -Recurse -Force }
New-Item -ItemType Directory -Path $out -Force | Out-Null

& dotnet restore $project -r win-x64
if ($LASTEXITCODE -ne 0) { throw 'dotnet restore failed.' }

& dotnet publish $project -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true --no-restore -o $out
if ($LASTEXITCODE -ne 0) { throw 'dotnet publish failed.' }

$exe = Join-Path $out 'SleepySource.exe'
if (-not (Test-Path $exe)) { throw 'SleepySource.exe was not produced.' }
if (Test-Path (Join-Path $out 'SleepySource.Engine.exe')) { throw 'Legacy SleepySource.Engine.exe must not be present.' }
if (-not (Test-Path (Join-Path $out 'web\index.html'))) { throw 'Published WebView2 UI is missing.' }
if (-not (Test-Path (Join-Path $out 'assets\app.ico'))) { throw 'Published application icon is missing.' }

Write-Host "SleepySource 1.0.0 published successfully: $out"
