# Preview Panel + Dev Process (Updated Plan)

Date: 2026-01-21

## Goals
- When user clicks **Preview** in the session top bar:
  - **Left column hidden immediately**.
  - **Right column stays visible initially** to show a new **Dev Server** terminal tab with live logs.
  - Dev script logs start streaming; the **Dev Server** tab is auto‑focused.
  - As soon as a localhost URL is detected in the logs, switch to **Preview layout** and **hide right column** (left already hidden).
- Preview layout includes an **address bar** (editable) and **Stop** button.
- Remove device buttons (desktop/tablet/mobile) from preview bar.

## Architecture Diagram (High‑Level)
```
[User Action]
  | click Preview button (Topbar)
  v
[UI State]
  preview.stage = "logs"
  preview.open = true
  rightPanel.activeTab = "dev-server"
  start dev process (if not running)
  |
  v
[Backend]
  StartProcess (kind=dev, session_id)
  -> ProcessRunner (agentctl)
  -> Workspace stream process.output
  -> Backend WS session.process.output
  |
  v
[Frontend]
  WS handler -> store.processes.outputsByProcessId
  |
  v
[Preview Hook]
  detect URL in dev output
  if found:
    preview.url = detected
    preview.stage = "preview"
    (left+right columns hidden)
  |
  v
[Preview Panel]
  iframe src = preview.url
  address bar editable
  stop button -> StopProcess
```

## UI States
We’ll model preview as **staged**:
- `closed`: normal 3‑column layout.
- `logs`: left hidden, right visible; **Dev Server** tab focused; logs streaming; preview not yet loaded.
- `preview`: left + right hidden; center + preview panel only; address bar + stop button visible.

## Store / Types
### New/Updated Store Types (apps/web/lib/state/store.ts)
```
export type PreviewStage = 'closed' | 'logs' | 'preview';

export type PreviewPanelState = {
  openBySessionId: Record<string, boolean>; // existing
  viewBySessionId: Record<string, 'preview' | 'output'>; // existing, keep for log view
  deviceBySessionId: Record<string, 'desktop' | 'tablet' | 'mobile'>; // keep for now but unused

  stageBySessionId: Record<string, PreviewStage>;
  urlBySessionId: Record<string, string>;
  urlDraftBySessionId: Record<string, string>; // for address bar editing
};

export type RightPanelState = {
  activeTabBySessionId: Record<string, string>; // e.g. 'commands', 'terminal-1', 'dev-server'
};
```

### New Store Actions
```
setPreviewStage(sessionId, stage)
setPreviewUrl(sessionId, url)
setPreviewUrlDraft(sessionId, url)
setRightPanelActiveTab(sessionId, tab)
```

## Data Flow (Low‑Level)
### 1) Preview Button Click
1. Topbar preview toggle is clicked.
2. Actions:
   - `setPreviewOpen(sessionId, true)`
   - `setPreviewStage(sessionId, 'logs')`
   - `setRightPanelActiveTab(sessionId, 'dev-server')`
   - `startProcess(kind='dev')` **if no dev process running**

### 2) Dev Output → URL Detection
1. WS handler receives `session.process.output` for dev process.
2. `appendProcessOutput(processId, chunk.data)` updates store.
3. `usePreviewPanel` listens to dev output changes; regex scans for `http://localhost` or `127.0.0.1`.
4. On first URL:
   - `setPreviewUrl(sessionId, url)`
   - `setPreviewUrlDraft(sessionId, url)`
   - `setPreviewStage(sessionId, 'preview')`

### 3) Layout Switch
- `TaskLayout` reads `preview.stageBySessionId`:
  - `logs`: hide left column; show center + right panel (with dev logs tab focused).
  - `preview`: hide both left and right columns; show center + preview panel only.

### 4) Address Bar
- Preview panel shows input bound to `urlDraftBySessionId`.
- On submit/blur, commit to `urlBySessionId`.
- `iframe.src = urlBySessionId`.

### 5) Stop Button
- `stopProcess(processId)` (dev process)
- Optional: set stage back to `logs` (keep logs) or `closed` (if desired).

## UI Components / File‑Level Plan

### `apps/web/components/task/task-layout.tsx`
- Add `previewStage` from store.
- If `previewOpen`:
  - Stage `logs`: render **center + right panel** (left hidden).
  - Stage `preview`: render **center + preview panel** (left+right hidden).

### `apps/web/components/task/task-right-panel.tsx`
- Add a **Dev Server** tab (value `dev-server`).
- Add new panel content (use new `ProcessTerminal` or output component bound to dev process output).
- Allow external control of active tab via prop or store (right panel state).

### `apps/web/components/task/preview/preview-panel.tsx`
- Remove device buttons.
- Add address bar (input) + Stop button.
- Show “Logs”/“Preview” toggle only if still needed; otherwise keep **Preview** and **Logs** views.

### `apps/web/hooks/use-preview-panel.ts`
- Extend to:
  - auto‑start dev process on preview open (if not running)
  - update stage to `logs`
  - detect URL from dev output and switch to `preview`
  - expose `previewUrl`, `previewUrlDraft`, `setPreviewUrlDraft`, `setPreviewUrl`

### `apps/web/lib/state/store.ts`
- Add preview stage + url fields
- Add right panel active tab state + actions

### Terminal for Dev Logs
Options:
1) **New ProcessTerminal** (preferred): uses `outputsByProcessId` from store and renders in xterm.
2) **Simple log view** (fallback): preformatted text with scroll.

## Edge Cases
- Dev script missing: keep current “Configure dev script” message.
- Dev process already running: don’t start a second; focus logs tab and detect URL from existing output.
- URL changes mid‑run: keep last detected URL, allow manual edit.
- Stop button: if stop fails, keep stage; allow retry.

## Minimal Backend Changes
- None for UI flow (existing process output stream already used).
- Ensure dev process outputs stream to WS (already implemented).

## Acceptance Checklist
- Clicking Preview hides left column, focuses Dev Server tab, logs stream.
- URL detected → preview layout shown (left + right hidden).
- Address bar editable + Stop button functional.
- Device buttons removed.
```
