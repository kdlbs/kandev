---
status: active
system: auth
created: 2026-08-15
owners:
  - tbd
---
# Trusted Proxies for X-Forwarded-For Requirements

## Overview

Behind a reverse proxy, Kandev records the proxy's IP as the login session IP (Settings > Account > Security) and keys the login rate limiter on it, even when the proxy forwards the real client in `X-Forwarded-For`. Operators who front Kandev with a proxy have no way to make the recorded IP match the actual client.

## Requirements

### REQ-AUTH-TRUSTED-PROXIES-001: Trusted Proxies for X-Forwarded-For

**Intent:** Behind a reverse proxy, Kandev records the proxy's IP as the login session IP (Settings > Account > Security) and keys the login rate limiter on it, even when the proxy forwards the real client in `X-Forwarded-For`. Operators who front Kandev with a proxy have no way to make the recorded IP match the actual client.

#### Acceptance criteria

- **AC-AUTH-TRUSTED-PROXIES-001.1:** Kandev SHALL read `server.trustedProxies` from startup YAML as a list of IP addresses or CIDR ranges. The `KANDEV_TRUSTED_PROXIES` environment variable remains a comma-separated override for the same setting.
- **AC-AUTH-TRUSTED-PROXIES-001.2:** The environment value SHALL override the YAML value when both are present.
- **AC-AUTH-TRUSTED-PROXIES-001.3:** When the TCP peer of a request is in the trusted list, the client IP SHALL resolve from the first valid forwarded-IP header in gin's configured order (`X-Forwarded-For`, then `X-Real-IP`), falling back to the peer only when neither header yields a valid address.
- **AC-AUTH-TRUSTED-PROXIES-001.4:** When the configured list is unset or empty, or the peer is not in the list, the client IP SHALL be the TCP peer; forwarded headers are ignored.
- **AC-AUTH-TRUSTED-PROXIES-001.5:** When any entry fails to parse as an IP or CIDR, startup SHALL log a warning naming the bad value(s) and the whole configured list SHALL be ignored (fail closed; no partial trust). Startup SHALL NOT fail.
- **AC-AUTH-TRUSTED-PROXIES-001.6:** The resolved client IP feeds the same consumers as today: the session IP recorded for login, setup, invite acceptance, and plugin-provided SSO (`AuthenticateExternal`), and the login rate-limiter key.
- **AC-AUTH-TRUSTED-PROXIES-001.7:** **GIVEN** `server.trustedProxies` unset and `KANDEV_TRUSTED_PROXIES` unset, **WHEN** a login request arrives from a proxy carrying `X-Forwarded-For: <client>`, **THEN** the session IP is the proxy's address (the TCP peer).
- **AC-AUTH-TRUSTED-PROXIES-001.8:** **GIVEN** `server.trustedProxies: [10.0.0.0/8]` and a request whose TCP peer is `10.0.0.5`, **WHEN** the request carries `X-Forwarded-For: 203.0.113.7`, **THEN** the login session IP is `203.0.113.7`.

## Migrated source detail

## Why

Behind a reverse proxy, Kandev records the proxy's IP as the login session IP
(Settings > Account > Security) and keys the login rate limiter on it, even
when the proxy forwards the real client in `X-Forwarded-For`. Operators who
front Kandev with a proxy have no way to make the recorded IP match the actual
client.

## What

- Kandev SHALL read `server.trustedProxies` from startup YAML as a list of IP
  addresses or CIDR ranges. The `KANDEV_TRUSTED_PROXIES` environment variable
  remains a comma-separated override for the same setting.
- The environment value SHALL override the YAML value when both are present.
- When the TCP peer of a request is in the trusted list, the client IP SHALL
  resolve from the first valid forwarded-IP header in gin's configured order
  (`X-Forwarded-For`, then `X-Real-IP`), falling back to the peer only when
  neither header yields a valid address.
- When the configured list is unset or empty, or the peer is not in the list, the
  client IP SHALL be the TCP peer; forwarded headers are ignored.
- When any entry fails to parse as an IP or CIDR, startup SHALL log a warning
  naming the bad value(s) and the whole configured list SHALL be ignored (fail
  closed; no partial trust). Startup SHALL NOT fail.
- The resolved client IP feeds the same consumers as today: the session IP
  recorded for login, setup, invite acceptance, and plugin-provided SSO
  (`AuthenticateExternal`), and the login rate-limiter key.

## Failure modes

- **Invalid entries.** A warning names the bad values and the configured list is
  ignored entirely; `X-Forwarded-For` is never trusted and the recorded IP
  stays the TCP peer. No crash.
- **No valid forwarded-IP header** on a request from a trusted peer: gin
  falls back to the TCP peer (only when neither `X-Forwarded-For` nor
  `X-Real-IP` yields a valid address).
- **Spoofing.** An operator who configures the list while the backend is
  directly reachable lets any caller whose peer address falls inside a
  configured trusted IP/CIDR forge the client IP, including the login
  rate-limiter key. Callers outside every configured range still fall back to
  their own peer address. This is a documented tradeoff; network placement
  stays the operator's responsibility.

## Scenarios

- **GIVEN** `server.trustedProxies` unset and `KANDEV_TRUSTED_PROXIES` unset,
  **WHEN** a login request arrives
  from a proxy carrying `X-Forwarded-For: <client>`, **THEN** the session IP
  is the proxy's address (the TCP peer).
- **GIVEN** `server.trustedProxies: [10.0.0.0/8]` and a request whose TCP peer
  is `10.0.0.5`, **WHEN** the request carries `X-Forwarded-For: 203.0.113.7`,
  **THEN** the login session IP is `203.0.113.7`.
- **GIVEN** `server.trustedProxies: [192.168.0.0/16]` and a request whose TCP
  peer is `10.0.0.5`, **WHEN** the request carries `X-Forwarded-For:
  203.0.113.7`, **THEN** the session IP is `10.0.0.5` (header ignored).
- **GIVEN** `server.trustedProxies: [10.0.0.0/8, not-a-cidr]`, **WHEN** the
  backend starts, **THEN** a startup warning names `not-a-cidr` and
  `X-Forwarded-For` is ignored entirely.
- **GIVEN** `KANDEV_TRUSTED_PROXIES=10.0.0.0/8` and a backend reachable
  directly, **WHEN** a caller whose peer address is `10.0.0.6` (inside the
  trusted range) sends `X-Forwarded-For: <forged>`, **THEN** the forged IP is
  recorded (documented operator tradeoff).
- **GIVEN** `KANDEV_TRUSTED_PROXIES=10.0.0.0/8` and a backend reachable
  directly, **WHEN** a caller whose peer address is `203.0.113.99` (outside
  every trusted range) sends `X-Forwarded-For: <forged>`, **THEN** the forged
  header is ignored and the session IP is `203.0.113.99`.
- **GIVEN** a trusted peer, **WHEN** a request carries no `X-Forwarded-For`
  and no `X-Real-IP` header, **THEN** the session IP is the TCP peer.
- **GIVEN** a trusted peer, **WHEN** a request carries only
  `X-Real-IP: 203.0.113.7`, **THEN** the session IP is `203.0.113.7`.
- **GIVEN** a trusted peer, **WHEN** a request carries
  `X-Forwarded-For: not-an-ip` and `X-Real-IP: 203.0.113.7`, **THEN** the
  session IP is `203.0.113.7` (gin skips the malformed header and reads the
  next configured one).

## Out of scope

- No change to `X-Forwarded-Proto` handling (session cookie secure flag,
  update checks), WebSocket `remote_addr` logging, or `RequestLogger` fields.
- No proxy auto-detection, no launcher CLI flag, no UI changes.
- No per-consumer overrides: the trusted-proxy list applies to every
  `ClientIP()` consumer, exactly as the unconfigured default does today.
