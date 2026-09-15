# Agent tab close behavior design

`UserSettings.AgentTabCloseBehavior` is normalized and persisted by the backend JSON settings payload. DTO, boot state, HTTP, and WebSocket settings paths project the effective value. The frontend uses the common settings mapper.

The hide path stores environment-scoped session IDs in session storage with task ownership. Dockview synchronization excludes hidden panels; reopening a session clears its record before adding the panel. This device-local layout state follows ADR 0041 while the choice of close behavior remains portable.
