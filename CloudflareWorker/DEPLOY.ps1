$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
Write-Host "Deploying SleepySource Auth Worker..." -ForegroundColor Cyan
Write-Host "Worker: sleepysource-api"
Write-Host "D1: sleepysource-auth"
Write-Host "Durable Object: REALTIME -> SleepySourceRealtime (SQLite)"
Write-Host ""
npx --yes wrangler@latest deploy
if ($LASTEXITCODE -ne 0) { throw "Wrangler deploy failed with exit code $LASTEXITCODE" }
Write-Host ""
Write-Host "Deployment complete." -ForegroundColor Green
Write-Host "Health check: https://sleepysource-api.stinkybud49.workers.dev/health"
