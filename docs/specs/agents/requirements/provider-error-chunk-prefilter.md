---
status: draft
system: agents
created: 2026-11-14
owners:
  - kandev
---

# Provider Error Classification Pre-Check on Message Chunks Requirements

## Overview

The ACP adapter marks an assistant message chunk as a provider diagnostic
candidate when the provider-error classifier gives it a high-confidence,
fallback-allowing classification. The adapter ran the full rule engine on
every chunk, although almost every chunk is ordinary assistant prose that no
rule can match. On a synthetic load of 3 sessions and 600 chunks per second,
agentctl used 35% of one core with the per-chunk classification and 16% with
the pre-check.

The agents system owns the classifier and the ACP adapter.

## Terminology

- **Default classification** - the low-confidence phase result `Classify`
  returns when no rule matches.
- **Gate token** - a lowercase literal that every match of some catalogue rule
  must contain.

## Requirements

### REQ-AGENTS-PROVIDER-ERROR-PREFILTER-001: Skip classification only when no rule can match

**Intent:** Keep the rule engine off the per-chunk path for ordinary prose
without losing any classification.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.1:** `routingerr.MayClassify` shall
  return false only when `Classify` returns the default classification for the
  same input.
- **AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.2:** Gate tokens shall be derived
  from the rule catalogue. A rule whose pattern has a match path without a
  usable literal shall declare one token per path, and an undeclared rule shall
  turn the gate off so that every input is classified.
- **AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.3:** Structured evidence shall never
  be gated: a structured error, an HTTP or ACP status, an error kind, a provider
  code, an exit code, a managed runtime package, enabled keyword context, or an
  injected provider failure shall make `MayClassify` return true.
- **AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.4:** The ACP adapter shall skip
  `Classify` for a message chunk only when `MayClassify` is false, and the
  diagnostic-candidate marker shall stay the same as when every chunk is
  classified.
