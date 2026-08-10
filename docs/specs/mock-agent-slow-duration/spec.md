---
status: shipped
created: 2026-08-08
owner: codex
---

# Mock-agent slow command duration syntax

## Why

The mock agent's `/slow` command is used to keep a deterministic agent turn
active while testing queueing, cancellation, and session behavior. Users expect
an argument such as `/slow 60` to mean 60 seconds, but the current command
silently falls back to its five-second default because the parser requires an
explicit duration unit.

## What

- `/slow` with no duration keeps its existing five-second default.
- `/slow <positive number>` interprets the bare number as seconds. For example,
  `/slow 60` holds the turn for 60 seconds.
- `/slow <duration-with-unit>` continues to accept explicit Go duration forms,
  including `/slow 60s`, `/slow 500ms`, and `/slow 2m`.
- Missing, zero, negative, or malformed duration arguments continue to use the
  five-second default rather than failing the prompt.

## Command contract

The command is exposed through the mock agent's ACP available-command
advertisement as `slow`. Its prompt form is `/slow [duration]`. The resolved
duration controls the total simulated turn delay, and the response reports the
resolved duration using the existing mock-agent output format.

## Scenarios

- **GIVEN** the mock agent is ready, **WHEN** it receives `/slow 60`, **THEN**
  it simulates a 60-second turn rather than using the five-second default.
- **GIVEN** the mock agent is ready, **WHEN** it receives `/slow 60s`, **THEN**
  it simulates the same 60-second turn.
- **GIVEN** the mock agent is ready, **WHEN** it receives `/slow 500ms`, **THEN**
  it preserves the explicit 500-millisecond duration.
- **GIVEN** the mock agent is ready, **WHEN** it receives `/slow`, **THEN** it
  uses the five-second default.
- **GIVEN** the mock agent is ready, **WHEN** it receives `/slow 0`, a negative
  value, or malformed text, **THEN** it uses the five-second default.

## Out of scope

- Adding a `/send` command; `/send 60` is not a mock-agent command.
- Changing the default duration or the behavior of `/sleep`, `/background`, or
  other mock-agent commands.
- Changing the existing ACP command advertisement or its input hint.
- Adding persistence, configuration, or a feature flag for the duration.
