---
id: "01-declare-pi-remote-auth"
title: "Declare Pi remote credentials and configuration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PI-REMOTE-AUTH-001
  - REQ-AGENTS-PI-REMOTE-AUTH-002
acceptance_criteria:
  - AC-AGENTS-PI-REMOTE-AUTH-001.1
  - AC-AGENTS-PI-REMOTE-AUTH-001.2
  - AC-AGENTS-PI-REMOTE-AUTH-001.3
  - AC-AGENTS-PI-REMOTE-AUTH-002.1
  - AC-AGENTS-PI-REMOTE-AUTH-002.2
system_design:
  - ../../specs/agents/system-design/pi-remote-auth.md
---

# Task 01: Declare Pi Remote Credentials and Configuration

## Summary

Give `agents.PiACP` a `RemoteAuth` with the auth file and four API-key
variables and a `pi.config` portable bundle, label the bundle in every locale,
and forward `OPENROUTER_API_KEY` over SSH.

## In scope

- `pi_acp.go` and `pi_acp_test.go`.
- `sshRemoteAgentCredentialEnvKeys` and `executor_ssh_pi_env_test.go`.
- The bundle label key in `portable-config-bundles.tsx` and the locale files.
- `docs/public/executors.md`.

## Out of scope

- Reading `PI_CODING_AGENT_DIR`.
- Env variables of other agents that SSH does not forward.

## Acceptance

The unit tests in the plan pass, and `pnpm run i18n:check` reports every
locale complete.
