# SleepySource 1.0.0 Release — Worker deployment

The desktop release expects the matching hosted Worker at:

`https://sleepysource-api.stinkybud49.workers.dev`

## What this Worker owns

- Kick OAuth Authorization Code + PKCE start/callback/status flow
- branded `/connect/kick` handoff page before Kick authorization
- encrypted server-side Kick access and refresh token storage
- automatic token refresh
- connection revocation/disconnect
- Kick webhook signature verification
- managed Kick event subscriptions
- D1-backed reliable desktop delivery queue
- authenticated realtime WebSocket delivery through a Durable Object
- Stream Dashboard channel/category proxy operations

The desktop application does **not** need the Kick Client ID, Client Secret,
Kick access token, refresh token, redirect URI, webhook URL or Cloudflare
settings.

## Required Worker configuration

Keep these existing server-side settings/bindings configured:

- `KICK_CLIENT_ID`
- `KICK_CLIENT_SECRET` (secret)
- `KICK_REDIRECT_URI`
- `TOKEN_ENCRYPTION_KEY` (secret)
- D1 binding named `DB`
- Durable Object binding named `REALTIME` targeting exported class
 `SleepySourceRealtime`

The Kick application redirect URI must match the deployed Worker callback:

`https://sleepysource-api.stinkybud49.workers.dev/oauth/kick/callback`

Do not put the Client Secret or token-encryption key in the desktop source.

## Release routes

OAuth / connection:

- `GET|POST /oauth/kick/start`
- `GET /connect/kick` (branded SleepySource handoff page)
- `POST /oauth/kick/status`
- `GET /oauth/kick/callback`
- `POST /kick/connection/status`
- `POST /kick/connection/refresh`
- `POST /kick/connection/disconnect`

Managed events:

- `POST /kick/events/status`
- `POST /kick/events/sync`
- `POST /kick/events/delivery/poll`
- `POST /kick/events/delivery/ack`
- `GET /realtime/connect` (WebSocket upgrade)
- `POST /realtime/status`
- `POST /kick/webhook`

Stream Dashboard proxy:

- `POST /kick/channel/metadata`
- `POST /kick/categories/search`
- `POST /kick/channel/update`

## Required Kick OAuth scopes

The included Worker requests:

- `user:read`
- `channel:read`
- `channel:write`
- `events:subscribe`

## Deployment order

1. Back up the currently deployed Worker source/settings.
2. Replace the Worker module with `worker.js` while retaining the
 existing D1 and Durable Object bindings and secrets.
3. Deploy the Worker.
4. Open `/health` and verify the configured flags are true.
5. Launch SleepySource 1.0.0 Release and choose **Connect with Kick**.
6. Verify the branded SleepySource connect page opens first, then choose **Continue with Kick**.
7. Approve the Kick authorization in the browser.
8. Back in SleepySource verify Account, Kick Events and Realtime show healthy.
9. Test a chat message and an Alert Studio test/real event before relying on the
 build for a stream.

The Worker creates/updates its D1 schema from code. D1 remains the reliable
source of truth for event delivery; the WebSocket is the fast path.
