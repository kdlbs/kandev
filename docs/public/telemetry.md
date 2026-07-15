# Telemetry

Kandev includes **strictly opt-in** anonymous product telemetry. Nothing is
ever collected or sent unless you explicitly enable it, and two environment
kill switches disable it unconditionally. This page is the complete, exact
list of what can be sent — if an event is not in the tables below, Kandev
does not send it.

## How consent works

- **Off by default.** A fresh install sends nothing. The onboarding dialog
  asks once ("Help improve Kandev"); skipping the question keeps telemetry
  off.
- **Change anytime** in `Settings → System → Telemetry`.
- **Anonymous install ID.** When (and only when) you opt in, Kandev mints a
  random UUID. It is not derived from your hardware, hostname, or account,
  and it is deleted if you opt out.
- Consent is stored per install in Kandev's local database
  (`telemetry.consent` in the `settings` table).

## Kill switches

These are honoured unconditionally, before your stored preference is even
read. With either set, Kandev starts no telemetry machinery at all and the
consent prompt is hidden:

| Variable | Effect |
| --- | --- |
| `DO_NOT_TRACK=1` | Disables all telemetry ([consoledonottrack.com](https://consoledonottrack.com) convention) |
| `KANDEV_TELEMETRY=off` | Disables all telemetry (Kandev-specific; `false`, `0`, and `disabled` also work) |

Dev mode and e2e test runs force `KANDEV_TELEMETRY=off` via `profiles.yaml`,
so local development and CI never emit events.

## Inspecting what would be sent

Set `KANDEV_TELEMETRY_DEBUG=1` and every outgoing payload is logged locally
before it leaves the machine.

## What is collected

Every event carries only these context properties: Kandev version, OS
(`linux`/`darwin`/`windows`), CPU architecture, and deploy mode
(`local`/`docker`/`k8s`/`desktop`).

### Events

| Event | Trigger | Extra properties |
| --- | --- | --- |
| `telemetry_enabled` | You opt in | — |
| `install_heartbeat` | Opt-in, then once per day while running | — |
| `task_created` / `task_deleted` | A task is created/deleted | — |
| `agent_run_started` / `agent_run_completed` / `agent_run_failed` | An agent execution starts/finishes/fails | — |
| `turn_completed` | An agent turn finishes | — |
| `workspace_created` | A workspace is created | — |
| `automation_run_created` | An automation run is recorded | — |
| `ui_page_viewed` | A UI route is opened | `page`: route label with IDs stripped (e.g. `settings.system.telemetry`, `t.id`) |
| `ui_action` | An allowlisted UI action | `action`: short identifier |
| `feature_used` | An allowlisted feature is used | `feature`: short identifier |

Domain events are counted by name only — the internal payloads (which contain
titles, repository names, and similar) are never forwarded. UI events are
validated server-side against an allowlist; property values must be short
identifiers (`^[a-z0-9][a-z0-9_.:-]{0,63}$`), so free text cannot ride along
even from a modified client.

### Never collected

- Prompts, chat messages, code, diffs, or terminal output
- Task titles, repository names, branch names, file paths, or URLs
- Your name, email, account identifiers, or IP-based profiles
- Environment variables, secrets, or configuration values

## Where the data goes

Events are batched in memory and sent to an EU-hosted PostHog instance
(`https://eu.i.posthog.com`) in anonymous mode (no person profiles are
created). Delivery is fail-silent: if the endpoint is unreachable the batch
is dropped — telemetry never blocks or retries aggressively, and unsent
events are not persisted to disk.

Self-hosters can redirect or replace the sink with
`KANDEV_TELEMETRY_ENDPOINT` and `KANDEV_TELEMETRY_API_KEY`, or disable
everything with the kill switches above.
