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

## System design

The migrated technical source is split into [part 1](../system-design/secret-scope-transfer.md).
