---
status: current
system: platform
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
---

# Linux System Notification Operands

## Purpose and boundary

Platform owns the backend System provider's transport of notification text.
This design covers Linux `notify-send` dispatch, not the desktop shell's native
notifier or event selection. The existing [notification requirement](../requirements/notifications.md)
owns the observable contract; [ADR 0039](../../../decisions/0039-native-desktop-integration-boundary.md)
retains the separate desktop boundary.

## Requirement mapping

`REQ-PLATFORM-NOTIFICATIONS-001`, criteria .13-.15, map to the dispatch and
verification boundaries below.

## Dispatch and compatibility

`Service.HandleInboxItem` passes the inbox item title as the notification body
through `dispatchGenericNotification` and `SystemProvider.Send`. Other callers
also supply `providers.Message` text. `localizedSystemMessage` retains its
existing runtime-summary behavior before dispatch.

When `sendLinuxNotification` finds `notify-send`, keep the configured `-t`
timeout and optional `-i` icon arguments, then place the `--` option terminator
before the title and body operands. Execute directly through `runCommand`;
do not interpolate payload text into a shell or alter either operand. This
protects both an option-like title and an option-like body, including when the
parser continues recognizing options after a positional argument. `zenity`
fallback, WSL, Darwin and Windows dispatch remain unchanged.

`runCommand` continues to use the supplied context and its existing default
deadline when none is provided. A nonzero exit, start failure, or cancellation
remains an error returned to the caller. `SystemProvider.Send` and `Available`
retain the `KANDEV_E2E_MOCK` suppression gate. These boundaries do not promise
that an OS notification daemon will display a successfully submitted message.

## Verification and operational impact

Exercise the real provider/subprocess boundary with a temporary fake executable
that records argument boundaries and simulates failure and blocking; no live
desktop bus or machine notification is needed. The regression must reject a
missing terminator when either payload position is option-like, rather than
only checking a helper-generated string. Verify normal text, timeout and icon,
failure propagation, cancellation, and E2E suppression as controls. Preserve
the existing semantic and desktop notification contracts; there is no new
configuration, persistence, API, telemetry, or user-facing copy.
