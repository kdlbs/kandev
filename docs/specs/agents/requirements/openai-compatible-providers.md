---
status: draft
system: agents
created: 2026-08-31
owners:
  - kandev
---
# OpenAI-compatible AI providers Requirements

## Overview

Kandev has no first-class notion of an AI provider. An agent profile carries a
model, a fallback model, CLI passthrough and flags, a command prefix, dynamic
config options, and free-form environment variables. It has no provider base
URL and no first-class API-key field. Model routing is delegated entirely to
each agent CLI's own configuration file (`~/.codex/config.toml`, OpenCode
config, Gemini env).

The gap surfaces when a user wants to point an agent at a self-hosted,
OpenAI-compatible HTTP router (9router, LiteLLM, vLLM, a self-hosted
OpenRouter) instead of a first-party vendor endpoint. Today this requires
hand-writing a CLI config file outside Kandev, fighting each CLI's config
constraints one error at a time, and exporting the API key in the shell that
starts the backend because the profile-declared value does not reach the child
process reliably. The full failure log is recorded in
`docs/specs/openai-compatible-providers/problem-statement.md` (local working
note).

The agent system owns configured agent identities, profiles, and provider
capabilities, so it owns this contract. The UI surfaces the fields but the
contract stays useful independently of any one settings screen.

## Terminology

- **OpenAI-compatible provider:** An HTTP endpoint that implements the OpenAI
  REST surface (`/v1/chat/completions` and/or `/v1/responses`, `/v1/models`)
  with `Authorization: Bearer <key>` auth.
- **Provider primitive:** The base URL plus API-key reference declared on an
  agent profile that Kandev injects into the agent CLI it runs.
- **Provider injection:** The per-agent translation of the provider primitive
  into the concrete CLI configuration and environment that agent needs.

## Requirements

### REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001: Provider primitive on the agent profile

**Intent:** Give a profile a portable, Kandev-owned way to declare an
OpenAI-compatible endpoint and its credential, visible in the profile UI and
covered by Kandev secrets, instead of an invisible host config file.

**User story:** As an operator, I want to set a base URL and API key on an
agent profile, so that Kandev points that agent at my self-hosted router
without me editing CLI config files on the host.

#### Acceptance criteria

- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.1:** An agent profile can store a
  provider selection (`native` default, or `openai_compatible`), a provider
  base URL, and a reference to a Kandev secret holding the API key. The base
  URL and key reference are persisted with the profile and returned in its API
  projection; the key value itself is never returned.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.2:** When the selection is
  `openai_compatible`, the profile editor shows a base-URL field and an
  API-key secret field, and requires a non-empty base URL that parses as an
  absolute `http(s)` URL before the profile can be saved.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.3:** The provider section is only
  offered for agents that advertise OpenAI-compatible provider support. For
  other agents the fields are hidden and any stored values are inert.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.4:** When the selection is
  `native`, profile behavior is byte-identical to today: no provider
  configuration or environment is injected.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.5:** A model identifier
  containing a slash is rejected at save time for an `openai_compatible`
  profile, with a message explaining that the target CLI routes slash-prefixed
  models to its built-in vendor provider.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.6:** When an API-key secret is
  referenced, the base URL must be `https` or an explicit loopback host
  (`localhost`, `127.0.0.0/8`, `::1`); a cleartext `http` URL to any other host
  is rejected at save time and again at launch, so the bearer key is never put
  on the wire in the clear. A URL with no key reference keeps the plain
  absolute-`http(s)` rule. Kandev validates the configured URL only; how the
  agent's own HTTP client follows redirects is outside Kandev's control and is
  not a substitute for pointing the profile at an `https` endpoint.

### REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002: Injection into the live agent session

**Intent:** Kandev, not the user, produces the CLI configuration that makes the
agent send its inference traffic to the declared endpoint with the declared
key, for the agents that support a generic OpenAI-compatible provider.

**User story:** As an operator, I want a task started on an
`openai_compatible` profile to reach my router, so that I do not maintain
`~/.codex/config.toml` by hand.

#### Acceptance criteria

- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.1:** When a session starts on an
  `openai_compatible` profile for a supporting agent, Kandev injects the
  provider base URL, a non-reserved provider identifier, the required wire
  protocol, and the API key into the agent subprocess without requiring a
  pre-existing host CLI config file.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.2:** The injected provider
  identifier never collides with a reserved built-in provider ID of the target
  CLI.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.3:** Injection is additive and
  scoped to the launched subprocess: it does not write, move, or delete any
  file under the user's home directory, and a `native` profile for the same
  agent is unaffected within the same Kandev process.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.4:** Injection works across the
  standalone and Docker executors. Any file Kandev needs to materialize for
  injection is created inside the session's own isolated agent home, not the
  shared host home.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.5:** When injection cannot be
  completed (missing secret, unresolvable base URL), the session start fails
  with a specific, sanitized error naming the provider misconfiguration, and
  does not fall back to the vendor endpoint silently.

### REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003: Provider reach for probe and utility inference

**Intent:** The model-capability probe and the one-shot inference/utility path
(`review`, objective assessment, generated titles) must reach the same
endpoint, so an `openai_compatible` profile is usable end to end and its model
list resolves.

**User story:** As an operator, I want `review` and the model picker to work on
my router-backed profile, so that the provider primitive is not Codex-chat only.

#### Acceptance criteria

- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003.1:** `openai_compatible` profiles
  take their model as free-text (AC-001.2), so no per-profile model probe is
  required for the model picker to work. The sessionless ACP probe executor
  nonetheless accepts `ProviderGatewayAuth` and applies the same gateway
  `authenticate` as a live session, so a probe that is later run for a
  provider-profile context reaches the declared endpoint with no further work.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003.2:** The one-shot inference
  executor applies the same provider injection, so a profile-scoped utility
  prompt reaches the declared endpoint with the declared key.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003.3:** A probe or inference call
  that fails against the provider surfaces a sanitized upstream status
  (for example an authentication failure) rather than a generic
  "peer disconnected" message.

### REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004: Predictable delivery of a profile-declared credential

**Intent:** A key declared on the profile must reach the agent process
deterministically. Two current behaviors defeat that and are corrected here.

**User story:** As an operator, I want the profile's API key to be the key the
agent uses, so that I do not have to export it in the shell that launches
Kandev.

#### Acceptance criteria

- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004.1:** When one secret-backed
  profile environment entry fails to resolve, the other resolvable entries are
  still delivered to the subprocess; only the failing entry is dropped, and the
  failure is logged with the offending key. A required provider key that fails
  to resolve still fails the launch per
  AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.5.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004.2:** A provider API key that
  Kandev injects for an `openai_compatible` profile takes precedence over an
  inherited process environment variable of the same name for that subprocess.
- **AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004.3:** Non-provider profile
  environment entries keep their current "fill missing only" precedence;
  the precedence change is scoped to the keys Kandev owns for the provider.

## Out of scope

- A provider catalog, per-workspace provider credentials, or a provider
  management screen separate from the agent profile.
- Non-OpenAI wire protocols (Anthropic-native, Gemini-native) and non-Bearer
  auth schemes.
- Agents that do not expose a generic OpenAI-compatible provider in their CLI
  configuration. Support is added per agent behind the capability flag.
- The worktree required-base-branch-refresh hard failure when the workspace
  repo has no working `origin` (problem-statement item 4). Tracked separately
  by the worktree owner.
- Cost, quota, or usage attribution for provider-routed traffic.
