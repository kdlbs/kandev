---
status: active
system: agents
created: 2026-09-30
owners:
  - kandev
---

# MiniMax Code requirements

## Overview

Users need to select MiniMax Code as a native agent and run tasks with its
subscription login. The agents system owns identity, discovery, profiles,
models, authentication and recovery, including their settings projection.

## Requirements

### REQ-AGENTS-MINIMAX-001: Native agent and setup

#### Acceptance criteria

- **AC-AGENTS-MINIMAX-001.1:** Settings shall expose `minimax-acp` as MiniMax,
  with discovery requiring a working `mcode` executable. Having npm alone
  shall not report it installed.
- **AC-AGENTS-MINIMAX-001.2:** Install shall use the official
  `@minimax-ai/code` package. Setup shall expose native login and explain
  mainland China and Global account commands on desktop and phone.
- **AC-AGENTS-MINIMAX-001.3:** Missing authentication shall remain visible as
  login required. Successful login and refresh shall populate native models
  without switching account or provider on failure.

### REQ-AGENTS-MINIMAX-002: Profiles and native protocol

#### Acceptance criteria

- **AC-AGENTS-MINIMAX-002.1:** Structured tasks and one-shot inference shall
  run native MiniMax Code through ACP. Models, variants, modes and configuration
  options shall come from the native session discovery, preserving their IDs.
  Unverified limits or media support shall not be advertised.
- **AC-AGENTS-MINIMAX-002.2:** A profile's chosen native provider/model/variant
  shall survive save and launch. Terminal passthrough shall translate ACP
  model IDs into MiniMax's CLI syntax, preserving resume and prompt arguments.
- **AC-AGENTS-MINIMAX-002.3:** MCP configuration, tool permissions,
  cancellation and session load shall use established ACP paths. Kandev shall
  not enable a broader permission mode in the CLI by default.

### REQ-AGENTS-MINIMAX-003: Credential and executor ownership

#### Acceptance criteria

- **AC-AGENTS-MINIMAX-003.1:** Native login shall own credential persistence;
  Kandev shall not parse, log, rewrite or automatically relocate OAuth tokens.
- **AC-AGENTS-MINIMAX-003.2:** Executor sessions shall use their own persistent
  MiniMax data directory. Ambient data/profile overrides shall not escape the
  executor home. Remote executors shall authenticate within that environment.

## Out of scope

OpenCode wrappers, new transports, static third-party API credentials,
Office provider-routing expansion, media inputs unsupported by native ACP,
and automatic cross-executor OAuth credential relocation.
