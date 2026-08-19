$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

Write-Host "SleepySource API 1.0.0 deployment" -ForegroundColor Cyan
Write-Host "Worker: sleepysource-api"
Write-Host "D1: sleepysource-auth (binding DB)"
Write-Host "Durable Object: SleepySourceRealtime (binding REALTIME)"
Write-Host "Compatibility date: 2026-08-17"
Write-Host "Workers Logs: enabled (invocation logs + dashboard persistence)"
Write-Host ""

$worker = Get-Content -Raw -LiteralPath ".\worker.js"
$config = Get-Content -Raw -LiteralPath ".\wrangler.jsonc"

$workerHosts = @([regex]::Matches($worker, '(?i)(?:https|wss)://sleepysource-api\.[a-z0-9.-]+\.workers\.dev') | ForEach-Object { $_.Value } | Select-Object -Unique)
$unexpectedHosts = @($workerHosts | Where-Object { $_ -notmatch '^(?:https|wss)://sleepysource-api\.sleepyservices\.workers\.dev$' })
if ($unexpectedHosts.Count -gt 0) {
    throw "Safety check failed: unexpected SleepySource API Workers hostname found: $($unexpectedHosts -join ', ')"
}
if ($worker -notmatch 'sleepysource-api\.sleepyservices\.workers\.dev') {
    throw "Safety check failed: sleepyservices hostname was not found. Deployment cancelled."
}
if ($worker -notmatch 'export class SleepySourceRealtime extends DurableObject') {
    throw "Safety check failed: SleepySourceRealtime export was not found. Deployment cancelled."
}
if ($worker -notmatch 'deleteKickEventSubscriptions') {
    throw "Safety check failed: Kick subscription refresh helper was not found. Deployment cancelled."
}
if ($config -notmatch '"observability"\s*:\s*\{') {
    throw "Safety check failed: observability configuration is missing. Deployment cancelled."
}
if ($config -notmatch '"invocation_logs"\s*:\s*true') {
    throw "Safety check failed: invocation logging is not enabled. Deployment cancelled."
}
if ($config -notmatch '"persist"\s*:\s*true') {
    throw "Safety check failed: Workers Logs dashboard persistence is not enabled. Deployment cancelled."
}

Write-Host "Running Wrangler dry-run first..." -ForegroundColor Yellow
npx --yes wrangler@4.124.0 deploy --dry-run --keep-vars
if ($LASTEXITCODE -ne 0) { throw "Wrangler dry-run failed. Nothing was deployed." }

Write-Host ""
Write-Host "Dry-run passed. Existing dashboard variables and secrets are preserved." -ForegroundColor Green
$answer = Read-Host "Type DEPLOY to update the live sleepysource-api Worker"
if ($answer -cne 'DEPLOY') {
    Write-Host "Cancelled. Nothing was deployed." -ForegroundColor Yellow
    exit 0
}

npx --yes wrangler@4.124.0 deploy --keep-vars
if ($LASTEXITCODE -ne 0) { throw "Deployment failed." }

Write-Host ""
Write-Host "Deployment command completed." -ForegroundColor Green
Write-Host "Health check: https://sleepysource-auth-page.pages.dev/api/health"
Write-Host "Logs: Cloudflare Workers & Pages > sleepysource-api > Observability"
