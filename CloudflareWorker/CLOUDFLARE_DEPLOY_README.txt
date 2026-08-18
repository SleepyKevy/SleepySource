SleepySource 1.0.0 Release — Cloudflare deployment package
============================================================

This package preserves the existing Cloudflare resources:

Worker name:
 sleepysource-api

D1 binding:
 DB -> sleepysource-auth
 Database ID: 506132fb-ed3d-4ffb-9e47-ad510050363a

Durable Object binding:
 REALTIME -> SleepySourceRealtime

Durable Object storage:
 SQLite

The exports declaration is intentionally included because this Durable Object
namespace is already provisioned in Cloudflare. Do not delete the namespace.

DEPLOY
------
1. Extract this ZIP to a normal folder.
2. Open PowerShell in the extracted folder.
3. Run:

 powershell -ExecutionPolicy Bypass -File .\DEPLOY.ps1

4. If Wrangler asks you to log in to Cloudflare, complete the browser login.
5. Wait for the deployment to finish.
6. Open:

 https://sleepysource-api.stinkybud49.workers.dev/health

The config uses keep_vars=true so dashboard-managed non-secret variables are
preserved. Existing Worker secrets are also preserved by Wrangler deployments.

Expected first line of worker.js:
 import { DurableObject } from "cloudflare:workers";

Do not add a legacy Durable Object migrations block. This package uses the
current declarative exports configuration.
