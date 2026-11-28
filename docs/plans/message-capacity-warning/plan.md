---
status: implemented
created: 2026-11-24
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
system_design:
  - ../../specs/agents/system-design/message-capacity-warning.md
---

# Explain capacity-deferred chat messages

Deliver a single vertical change under
[bounded message turns](../../specs/agents/requirements/session-concurrency-ceiling.md).
The risk is attributing every queued send to capacity. Carry the specific
admission reason from the queue write to chat and verify both positive and
negative cases.

## Work orders

- [x] [01. Surface the capacity reason in chat](task-01-capacity-notice.md)

## ASCII UI preview

UI-01. Desktop and phone chat after Send, with the queue collapsed.

```text
[clock] Message queued because the session limit was reached.
    The current session limit is 1.
    It will send automatically when capacity is available.
[1 queued]
[Compose a message ...]
```

Use the shared yellow warning surface in the existing composer region and transcript scroll owner. Phone text wraps
above the same queue control. The explanation requires no tap or hover.
