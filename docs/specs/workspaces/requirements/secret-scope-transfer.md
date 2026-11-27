---
status: active
system: workspaces
created: 2026-08-11
owners:
  - tbd
---
# Copy and Move Secrets Between Scopes Requirements

## Overview

Moving a credential between a workspace and the user's Global set (or between workspaces) currently means reveal, copy the plaintext, create a new secret, and delete the old one. Users must handle the value outside Kandev, which is both tedious and leak-prone. Kandev should copy or move a secret across scopes as one operation, without ever showing the value.

## Requirements

### REQ-WORKSPACES-SECRET-SCOPE-TRANSFER-001: Copy and Move Secrets Between Scopes

**Intent:** Moving a credential between a workspace and the user's Global set (or between workspaces) currently means reveal, copy the plaintext, create a new secret, and delete the old one. Users must handle the value outside Kandev, which is both tedious and leak-prone. Kandev should copy or move a secret across scopes as one operation, without ever showing the value.

#### Acceptance criteria

- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.1:** Every secret row on the Global secrets page (`/settings/general/secrets`) and on a Workspace secrets page (`/settings/workspace/:id/secrets`) offers a **Copy/Move** action.
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.2:** The action opens one dialog that works the same on both pages. It contains:
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.3:** a **Copy / Move** radio (default Copy);
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.4:** a **destination** picker: **General** plus every workspace; when the source secret is Workspace-scoped its own workspace is excluded, and when the source is Global, General is excluded (a same-scope destination is a no-op);
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.5:** an editable **target name** field, pre-filled with `<name> (from Global)` for a Global source or `<name> (from <workspace name>)` for a Workspace source.
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.6:** **Copy** creates a new secret in the destination scope with the chosen name and the source's value. The source stays.
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.7:** **Move** copies the secret to the destination and then removes the source, as one atomic operation. The dialog states that the original will be removed from its current scope.
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.8:** Workspace-to-workspace copy/move is supported through the destination picker.

- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.9:** When a user leaves a workspace destination before its name lookup finishes, then returns to the same destination in the mounted Copy/Move dialog, the destination pre-check shall obtain current names. A result abandoned on the earlier visit shall not supply the current pre-check. Once the current lookup returns a matching trimmed target name, the existing inline conflict message and invalid name field shall appear, and Copy or Move shall be unavailable until the name is changed.
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.10:** A completed destination-name pre-check shall remain reusable on return to that workspace within the same transfer session while its single cached entry has not been invalidated by a different workspace read or explicit refresh. A new transfer session shall refresh destination names; switching workspaces shall not publish names from an abandoned lookup for another workspace. Global names shall continue to reflect the Global secret collection.
- **AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.11:** If the current destination-name lookup fails, the pre-check shall remain best-effort: it shall not invent a name conflict or prevent an otherwise valid submission solely because the lookup failed. The backend shall remain authoritative for atomic duplicate-name rejection, and the existing name-field conflict and generic transfer-failure handling shall remain available on desktop and phone.

## Exclusions

This recovery does not change transfer payloads, authorization, atomic copy/move behavior, secret values, destination selection rules, or dialog presentation. It does not introduce a new loading restriction on submission or a background polling/retry policy.

## System design

The migrated technical source is split into [part 1](../system-design/secret-scope-transfer.md).
