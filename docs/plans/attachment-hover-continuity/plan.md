---
status: implemented
created: 2026-11-10
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Attachment preview hover continuity

## Outcome

Keep composer attachment preview controls reachable while the pointer crosses
from the chip into the preview. This repairs the existing
AC-TASKS-PROMPT-ATTACHMENTS-001.15 contract. Upload and delivery behavior do not change.

## Delivery

[Task 01](task-01-preserve-hover-continuity.md) owns the correction and regression.
It has no dependencies. Use the existing Radix close timer with an explicit
300 ms delay; entering the preview cancels closure. Touch retains its drawer.

## ASCII UI preview

UI-01: Composer attachment preview, fine pointer, open state.

```text
[Preview image]
[Prompt] [File]
     ^ pointer crosses gap without closing preview
[Image attachment] [Remove]
[Message composer]
```

Control order and placement remain unchanged. Phone users tap the existing
chip to open the existing drawer. Only desktop pointer timing changes.

## Verification and results

Run the two targeted component files listed in the work order. Their regression
checks a 250 ms crossing, preview activation, and eventual closure. Live Chromium
also verifies vertical and diagonal crossings and Prompt/File controls. The
initial test failed on the immediate-close behavior; all 7 component tests pass
with the correction. Public documentation does not change for this repair.
