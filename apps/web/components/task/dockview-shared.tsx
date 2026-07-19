"use client";

import React, { useCallback, useEffect, useMemo } from "react";
import { useShallow } from "zustand/react/shallow";
import {
  DockviewDefaultTab,
  type IDockviewPanelProps,
  type IDockviewPanelHeaderProps,
} from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStore } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";

// Panel components (rendered via portals, not directly by dockview)
import { TaskChatPanel } from "./task-chat-panel";
import { TaskChangesPanel } from "./task-changes-panel";
import { ChangesPanel } from "./changes-panel";
import { FilesPanel } from "./files-panel";
import { TaskPlanPanel } from "./task-plan-panel";
import { FileEditorPanel } from "./file-editor-panel";
import { PassthroughToolbar } from "./passthrough-toolbar";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { ContextMenuTab } from "./tab-context-menu";
import { ChangesTab } from "./changes-tab";
import { PlanTab } from "./plan-tab";
import { PreviewFileTab, PreviewDiffTab, PreviewCommitTab, PinnedDefaultTab } from "./preview-tab";
import { SessionTab } from "./session-tab";
import { TerminalTab } from "./terminal-tab";
import { TerminalPanel } from "./terminal-panel";
import { BrowserPanel } from "./browser-panel";
import { VscodePanel } from "./vscode-panel";
import { CommitDetailPanel } from "./commit-detail-panel";
import type { OpenDiffOptions } from "./changes-diff-target";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";

import { setPanelTitle, panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { getWebSocketClient } from "@/lib/ws/connection";
import { usePortalSlot } from "@/lib/layout/panel-portal-host";
import { ENV_SCOPED_DOCKVIEW_COMPONENTS } from "@/lib/state/dockview-env-scoped-components";

// ---------------------------------------------------------------------------
// PORTAL SLOT — generic dockview component that adopts a persistent portal
// ---------------------------------------------------------------------------

/**
 * Components whose portals are tied to a specific task environment.
 *
 * When the user switches task envs, portals for these components are released
 * via `panelPortalManager.releaseByEnv()` so stale state (WebSocket
 * connections, iframes, editor buffers) from the old env doesn't leak
 * into the new one. Same-env session switches are a no-op — these portals
 * persist because the underlying workspace, container, and git history all
 * belong to the env, not the session.
 *
 * A component belongs here if its content is bound to env-specific runtime
 * state that can't be swapped by simply reading a new `activeSessionId` from
 * the store:
 *
 *  - file-editor   — editing a file in the env's worktree
 *  - browser       — iframe preview of the env's dev server URL
 *  - vscode        — VS Code Server iframe running in the env's container
 *  - commit-detail — displays a commit from the env's git history
 *  - diff-viewer   — shows file diffs from the env's working tree
 *  - pr-detail     — PR linked to the env's task
 *
 * Components NOT listed here are **global** — they read `activeSessionId`
 * reactively from the store and automatically reflect the current session:
 *
 *  - sidebar  — workspace/task navigation, not session-specific
 *  - chat     — subscribes to `activeSessionId`, re-renders for new session
 *  - terminal — uses `useEnvironmentSessionId()`, reconnects only on env change
 *  - changes  — uses `useEnvironmentSessionId()` for stable git state
 *  - files    — uses `useEnvironmentSessionId()` for stable file tree
 *  - plan     — reads `activeTaskId` from the store
 */
export const ENV_SCOPED_COMPONENTS = ENV_SCOPED_DOCKVIEW_COMPONENTS;

/**
 * Every entry in the dockview `components` map uses this wrapper.
 * It renders an empty container and attaches the persistent portal element
 * managed by PanelPortalManager.  The actual panel content is rendered by
 * PanelPortalHost outside the dockview tree.
 *
 * Env-scoped panels are tagged with the current task-env ID so they can be
 * cleaned up on env switch.
 */
function PortalSlot(props: IDockviewPanelProps) {
  const component = props.api.component;
  const activeEnvId = useAppStore((s) => {
    const sid = s.tasks.activeSessionId;
    return sid ? (s.environmentIdBySessionId[sid] ?? null) : null;
  });
  const envId = ENV_SCOPED_COMPONENTS.has(component) ? (activeEnvId ?? undefined) : undefined;
  const containerRef = usePortalSlot(props, envId);
  return <div ref={containerRef} className="h-full w-full overflow-hidden" />;
}

// --- COMPONENT MAP ---
// All panel types use the same PortalSlot wrapper — dockview only manages
// layout positioning.  Actual rendering happens in PanelPortalHost below.
export const dockviewComponents: Record<string, React.FunctionComponent<IDockviewPanelProps>> = {
  chat: PortalSlot,
  "diff-viewer": PortalSlot,
  "file-editor": PortalSlot,
  "commit-detail": PortalSlot,
  changes: PortalSlot,
  files: PortalSlot,
  terminal: PortalSlot,
  browser: PortalSlot,
  vscode: PortalSlot,
  plan: PortalSlot,
  "pr-detail": PortalSlot,
  // Backwards compat aliases for saved layouts
  "diff-files": PortalSlot,
  "all-files": PortalSlot,
};

// --- TAB COMPONENTS ---
function PermanentTab(props: IDockviewPanelHeaderProps) {
  return <DockviewDefaultTab {...props} hideClose />;
}

export const dockviewTabComponents: Record<
  string,
  React.FunctionComponent<IDockviewPanelHeaderProps>
> = {
  permanentTab: PermanentTab,
  changesTab: ChangesTab,
  planTab: PlanTab,
  sessionTab: SessionTab,
  terminalTab: TerminalTab,
  previewFileTab: PreviewFileTab,
  previewDiffTab: PreviewDiffTab,
  previewCommitTab: PreviewCommitTab,
  pinnedDefaultTab: PinnedDefaultTab,
};

export { ContextMenuTab };

// ---------------------------------------------------------------------------
// PORTAL CONTENT — the actual panel implementations rendered via portals
// ---------------------------------------------------------------------------

// Each content component renders the real panel UI.  They live permanently
// in the PanelPortalHost and survive dockview layout switches.

function useChatSessionTitle(panelId: string, sessionId: string | null, isSessionTab: boolean) {
  const sessionTitleData = useAppStore(
    useShallow((state) => {
      if (!sessionId) return null;
      const session = state.taskSessions.items[sessionId];
      if (!session) return null;
      return { name: session.name ?? null, agentProfileId: session.agent_profile_id ?? null };
    }),
  );
  const { agentProfiles } = useSettingsData(Boolean(sessionTitleData?.agentProfileId));
  const agentLabel = useMemo(() => {
    if (sessionTitleData?.name) return sessionTitleData.name;
    if (!sessionTitleData?.agentProfileId) return null;
    const profile = agentProfiles.find((p) => p.id === sessionTitleData.agentProfileId);
    if (!profile) return null;
    const parts = profile.label.split(" \u2022 ");
    return parts[1] || parts[0] || profile.label;
  }, [agentProfiles, sessionTitleData]);
  useEffect(() => {
    let label = "Agent";
    if (isSessionTab && agentLabel) {
      label = agentLabel;
    }
    setPanelTitle(panelId, label);
  }, [panelId, isSessionTab, agentLabel]);
}

function ChatContent({ panelId, params }: { panelId: string; params: Record<string, unknown> }) {
  const paramSessionId = params?.sessionId as string | undefined;
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = paramSessionId ?? storeSessionId;
  const taskId = useAppStore((state) => {
    if (sessionId) {
      return state.taskSessions.items[sessionId]?.task_id ?? state.tasks.activeTaskId;
    }
    return state.tasks.activeTaskId;
  });
  const { openFile } = useFileEditors();
  const isPassthrough = useAppStore((state) =>
    sessionId ? isPassthroughSession(state.taskSessions.items[sessionId]) : false,
  );
  useChatSessionTitle(panelId, sessionId, !!paramSessionId);

  if (isPassthrough) {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <PassthroughToolbar sessionId={sessionId} taskId={taskId} />
        </PanelBody>
      </PanelRoot>
    );
  }
  return (
    <TaskChatPanel
      sessionId={sessionId}
      taskId={sessionId ? taskId : null}
      onOpenFile={openFile}
      onOpenFileAtLine={openFile}
      hideSessionsDropdown
    />
  );
}

/**
 * Force a fresh git-status push whenever the diff panel becomes the active
 * dockview tab.
 *
 * Background: the diff panel's content is derived from `gitStatus` (the
 * per-file `.diff` string), which only refreshes when a `session.git.event`
 * status_update arrives from agentctl's workspace poll loop. That loop runs at
 * 3s (fast) only while the workspace is in fast poll mode; if the focus→fast
 * upgrade lost a race with agentctl startup the loop can sit in slow mode (30s)
 * and the open diff shows stale content until the next slow tick.
 *
 * This is the diff-side analog of `useResyncOnTabActivate` in
 * file-editor-panel.tsx (which force-syncs editor content on activation). Tab
 * activation is a deterministic, user-driven "I'm about to look at this diff"
 * signal, so we ask the backend for a fresh git-status snapshot via
 * `refreshSessionData` (re-sends `session.focus`, whose handler pushes a fresh
 * `GetGitStatusMultiFresh` result) — closing the WS-event-miss gap without
 * depending on poll cadence. No-op when the session isn't focused.
 */
function useResyncGitStatusOnTabActivate(panelId: string, sessionId: string | null) {
  useEffect(() => {
    if (!sessionId) return;
    const entry = panelPortalManager.get(panelId);
    if (!entry?.api) return;
    const refreshNow = () => {
      const client = getWebSocketClient();
      client?.refreshSessionData(sessionId);
    };
    // If the panel is already active when this effect first runs,
    // onDidActiveChange won't fire (no transition) — refresh immediately so the
    // initial open benefits from the same WS-event-miss recovery.
    if (entry.api.isActive) refreshNow();
    const disposable = entry.api.onDidActiveChange((event) => {
      if (event.isActive) refreshNow();
    });
    return () => disposable.dispose();
  }, [panelId, sessionId]);
}

function DiffViewerContent({
  panelId,
  params,
}: {
  panelId: string;
  params: Record<string, unknown>;
}) {
  const selectedDiff = useDockviewStore((s) => s.selectedDiff);
  const setSelectedDiff = useDockviewStore((s) => s.setSelectedDiff);
  const { openFile } = useFileEditors();
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const panelKind = (params?.kind as string) ?? "all";
  const selectedPath = panelKind === "file" ? (params?.path as string) : undefined;
  const selectedRepositoryName =
    panelKind === "file" ? (params?.repositoryName as string | undefined) : undefined;
  const panelSelectedDiff = panelKind === "all" ? selectedDiff : null;
  useResyncGitStatusOnTabActivate(panelId, activeSessionId);
  const handleClosePanel = useCallback(() => {
    const dockApi = useDockviewStore.getState().api;
    const panel = dockApi?.getPanel(panelId);
    if (dockApi && panel) dockApi.removePanel(panel);
  }, [panelId]);

  return (
    <TaskChangesPanel
      mode={panelKind as "all" | "file"}
      filePath={selectedPath}
      fileRepositoryName={selectedRepositoryName}
      selectedDiff={panelSelectedDiff}
      onClearSelected={() => setSelectedDiff(null)}
      onOpenFile={openFile}
      onBecameEmpty={handleClosePanel}
    />
  );
}

function ChangesContent({ panelId }: { panelId: string }) {
  const addDiffViewerPanel = useDockviewStore((s) => s.addDiffViewerPanel);
  const addFileDiffPanel = useDockviewStore((s) => s.addFileDiffPanel);
  const addCommitDetailPanel = useDockviewStore((s) => s.addCommitDetailPanel);
  const { openFile } = useFileEditors();

  // Dynamic title with file count — use environment-stable sessionId so the
  // tab title doesn't re-fetch on same-environment session tab switches.
  const activeSessionId = useEnvironmentSessionId();
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId);
  const fileCount = gitStatus?.files ? Object.keys(gitStatus.files).length : 0;
  const totalCount = fileCount + commits.length;

  useEffect(() => {
    const title = totalCount > 0 ? `Changes (${totalCount})` : "Changes";
    setPanelTitle(panelId, title);
  }, [totalCount, panelId]);

  const handleEditFile = useCallback(
    (path: string, repo?: string) => openFile(path, repo),
    [openFile],
  );
  const handleOpenDiffFile = useCallback(
    (path: string, options?: OpenDiffOptions) =>
      addFileDiffPanel(path, { source: options?.source, repositoryName: options?.repositoryName }),
    [addFileDiffPanel],
  );
  const handleOpenCommitDetail = useCallback(
    (sha: string, repo?: string) => addCommitDetailPanel(sha, { repo }),
    [addCommitDetailPanel],
  );
  const handleOpenDiffAll = useCallback(() => addDiffViewerPanel(), [addDiffViewerPanel]);
  const handleOpenReview = useCallback(() => {
    window.dispatchEvent(new CustomEvent("open-review-dialog"));
  }, []);

  return (
    <ChangesPanel
      onOpenDiffFile={handleOpenDiffFile}
      onEditFile={handleEditFile}
      onOpenCommitDetail={handleOpenCommitDetail}
      onOpenDiffAll={handleOpenDiffAll}
      onOpenReview={handleOpenReview}
    />
  );
}

function FilesContent() {
  const { openFile } = useFileEditors();
  const handleOpenFile = useCallback(
    (file: { path: string; name: string; content: string }) => openFile(file.path),
    [openFile],
  );
  return <FilesPanel onOpenFile={handleOpenFile} />;
}

function PlanContent() {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  return <TaskPlanPanel taskId={taskId} visible />;
}

// ---------------------------------------------------------------------------
// renderPanel — maps component names to their portal content
// ---------------------------------------------------------------------------

/** Resolve legacy component aliases to current names. */
const COMPONENT_ALIASES: Record<string, string> = {
  "diff-files": "changes",
  "all-files": "files",
};

function resolveComponent(component: string): string {
  return COMPONENT_ALIASES[component] ?? component;
}

export function renderPanel(
  panelId: string,
  component: string,
  params: Record<string, unknown>,
): React.ReactNode {
  const resolved = resolveComponent(component);

  switch (resolved) {
    case "sidebar":
      return null;
    case "chat":
      return <ChatContent panelId={panelId} params={params} />;
    case "diff-viewer":
      return <DiffViewerContent panelId={panelId} params={params} />;
    case "file-editor":
      return <FileEditorPanel panelId={panelId} params={params} />;
    case "commit-detail":
      return <CommitDetailPanel panelId={panelId} params={params} />;
    case "changes":
      return <ChangesContent panelId={panelId} />;
    case "files":
      return <FilesContent />;
    case "terminal":
      return <TerminalPanel panelId={panelId} params={params} />;
    case "browser":
      return <BrowserPanel panelId={panelId} params={params} />;
    case "vscode":
      return <VscodePanel panelId={panelId} />;
    case "plan":
      return <PlanContent />;
    case "pr-detail":
      return <PRDetailPanelComponent panelId={panelId} />;
    default:
      return <div className="p-4 text-muted-foreground">Unknown panel: {component}</div>;
  }
}

export const VALID_COMPONENTS = new Set(Object.keys(dockviewComponents));
