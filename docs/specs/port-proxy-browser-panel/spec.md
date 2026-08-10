---
status: shipped
created: 2026-08-07
owner: kandev
---

# Open proxy URLs in the browser panel

## Why

Users can copy a proxy URL or open it in the system browser. They cannot send the URL directly to
the Kandev Browser panel. This adds work when the user already has a Browser panel in the task
workbench.

## What

- Each proxy URL row in the Port Forwarding dialog has an **Open in browser panel** action on the
  desktop workbench.
- The action is present for proxy URLs only. Tunnel URL rows keep the current copy and system
  browser actions.
- When a Browser panel exists in any Dockview group, the action reuses that panel. It focuses the
  panel, replaces its address value with the proxy URL, and loads the proxy URL.
- When more than one Browser panel exists, the active Browser panel is reused. If no Browser panel
  is active, the first Browser panel in Dockview panel order is reused.
- When no Browser panel exists, the action creates one in the current central group, focuses it,
  inserts the proxy URL, and loads the proxy URL.
- The Port Forwarding dialog closes after the action succeeds. This makes the focused Browser panel
  visible.
- The action does not create a second Browser panel when one already exists.
- The existing copy action and system browser action continue to work.
- The action has an accessible name and a tooltip. Its desktop icon button has a touch-safe hit area
  when the dialog is used with a coarse pointer.
- Phone and tablet layouts do not have Dockview groups. They do not show this Dockview-only action.
  The existing system browser action remains available as the mobile fallback for the same proxy
  URL.

## State and data ownership

This feature adds no persistent data, database field, HTTP route, or WebSocket message.

- `PortForwardDialog` derives the proxy URL from the session ID and port.
- The Dockview store owns the current Dockview API and central group ID.
- The Browser panel owns its current URL in the Dockview panel parameter `params.url`. A parameter
  update is forwarded to the existing Browser panel portal, which updates its address field and
  iframe navigation.
- Existing Dockview environment-layout persistence may save the panel and its URL in the current
  browser session. This feature adds no new persistence rule. The URL is no longer available when
  the existing session-scoped proxy is no longer available.

## API surface

No new backend API is required. The feature reuses the existing session-scoped proxy URL and the
existing Dockview client API:

- `DockviewApi.panels` and `DockviewPanelApi.setActive()` locate and focus a Browser panel.
- `DockviewPanelApi.updateParameters({ url })` sends a new URL to an existing Browser panel.
- `DockviewApi.addPanel(...)` creates a Browser panel in the central group when needed.

## Failure modes

- If the Dockview API is not available, the Dockview-only action is not rendered. The existing copy
  and system browser actions remain available.
- If the central group ID is stale, the existing Dockview placement fallback selects a valid group.
- If the proxy page cannot load, the Browser panel still shows the requested URL and keeps the
  existing Browser panel loading behavior. The action does not retry a page load or show a new
  port-forwarding error.

## Persistence guarantees

- No feature-specific state survives a Kandev restart.
- If the existing Dockview environment layout saves the Browser panel, its URL follows that existing
  session-storage behavior. The feature does not copy the URL to the backend or to another task.
- Port discovery, tunnel state, and proxy authorization keep their current lifecycle.

## Scenarios

- **GIVEN** a desktop task with no Browser panel, **WHEN** the user clicks **Open in browser panel**
  on a proxy URL row, **THEN** one Browser panel opens in the central group, becomes active, shows
  the proxy URL in its address input, loads the URL, and the Port Forwarding dialog closes.
- **GIVEN** a desktop task with a Browser panel in a non-central group, **WHEN** the user clicks the
  action for a different proxy URL, **THEN** the existing panel remains in its group, becomes
  active, shows the new URL, and loads the new URL.
- **GIVEN** a desktop task with two Browser panels and one active Browser panel, **WHEN** the user
  clicks the action, **THEN** the active Browser panel is updated and no new panel is created.
- **GIVEN** a proxy URL row and a tunnel URL row, **WHEN** the user views their actions, **THEN**
  only the proxy row has **Open in browser panel** and both rows have copy and system browser
  actions.
- **GIVEN** a phone or tablet task, **WHEN** the user opens the Port Forwarding dialog, **THEN**
  the existing system browser action remains reachable, the dialog stays inside the viewport, and
  the document has no horizontal overflow.
- **GIVEN** the Dockview API is unavailable, **WHEN** the user views the Port Forwarding dialog,
  **THEN** the Dockview-only action is absent and the existing copy and system browser actions are
  still usable.

## Out of scope

- Adding a Browser panel to the phone layout or changing the tablet preview layout.
- Adding browser history, multiple URL tabs, or a new proxy transport.
- Changing proxy URL generation, tunnel behavior, port discovery, or authorization.
- Opening the proxy URL in a new system browser tab from the new action.
