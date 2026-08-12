# API reference

All responses are JSON. Production requests require HTTPS. User and device
credentials use `Authorization: Bearer <opaque token>`. Browser admin sessions
use an HttpOnly, Secure, SameSite=Strict cookie and an `X-CSRF-Token` header for
state changes.

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET | `/` | optional browser session | Redirect to admin login or console |
| GET | `/admin/login` | optional browser session | Login page; redirects authenticated admins |
| GET | `/admin` | administrator browser session | Admin console; redirects missing/expired sessions |
| POST | `/api/v1/auth/login` | none | Username/password login |
| POST | `/api/v1/auth/logout` | user | Revoke current session |
| GET | `/api/v1/me` | user | Current user |
| POST | `/api/v1/admin-handoff` | administrator | Create a 45-second, one-time browser handoff |
| POST | `/api/v1/maintenance` | administrator | Create one-use, device-bound local maintenance ticket |
| GET | `/admin/handoff?code=…` | one-time handoff | Redeem handoff into an admin cookie and redirect |
| GET | `/api/v1/connections` | user | Authorised, enabled connections only |
| POST | `/api/v1/connections/{id}/launch` | user | Create device-bound launch ticket |
| POST | `/api/v1/devices/enrol` | one-time token | Enrol device; bearer token returned once |
| POST | `/api/v1/agent/redeem-launch` | device | Redeem launch ticket once |
| POST | `/api/v1/agent/redeem-maintenance` | device | Redeem local maintenance ticket once |
| POST | `/api/v1/agent/heartbeat` | device | Last seen and client versions |
| POST | `/api/v1/agent/session-event` | device | Session lifecycle audit |
| GET/POST/PUT/DELETE | `/api/v1/admin/*` | administrator | Managed resources |

Launch request:

```json
{"device_identifier":"pi-living-room"}
```

Local agent request (one JSON object per Unix-socket connection):

```json
{"action":"launch","ticket":"opaque-ticket"}
```

Errors are stable and safe for users:

```json
{"error":{"code":"CONNECTION_NOT_AUTHORISED","message":"You are not authorised to launch this connection.","request_id":"..."}}
```

Admin resources are `dashboard`, `users`, `groups`, `connections`,
`credentials`, `devices`, `audit`, `policies`, `permissions`, `memberships`, and
`enrolment-tokens`. Stored secret values never appear in list responses.

Browser navigation routes return redirects. API routes never return HTML login
pages: an expired bearer/cookie session receives JSON 401, and the admin
JavaScript redirects the browser to `/admin/login`.

The launcher handoff endpoint returns a relative path, never a password or
browser cookie. The handoff value is stored only as a hash, expires after 45
seconds, can be redeemed once, and creates a browser session independent from
the in-memory launcher session.

Each permission can select a credential override. A launch resolves the direct
user permission first, then an applicable group permission, then the connection
default. Policy records control allowed weekdays, local-time windows, daily
minutes, per-session minutes, and timezone.
