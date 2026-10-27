---
status: current
system: platform
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
---

# Localized Update Notifications

## Boundary

The notification service owns the semantic release version and may provide
English fallback text for remote providers. The Local provider's browser and
native delivery derive their own copy from the active application locale when
that version is present. Remote provider delivery is unchanged.

## Requirement mapping

`AC-PLATFORM-NOTIFICATIONS-001.16` is met by formatting the known version with
existing client locale keys in `useUpdateAvailableToast` before dispatching the
same localized title and body to the in-app toast and native notification. If a
legacy event lacks a version, its supplied title and body remain the fallback.
Runtime update notifications retain their separate structured-copy path.

## Verification

Exercise a backend-shaped notification containing English fallback copy in a
pseudo locale through the toast hook and through the WebSocket browser path.
Verify native delivery uses the same localized copy and preserve runtime update
and legacy no-version behavior.
