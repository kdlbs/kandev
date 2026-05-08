"use client";

import React, { useCallback, useEffect, useRef, memo } from "react";
import {
  DockviewReact,
  DockviewDefaultTab,
  type IDockviewPanelProps,
  type IDockviewPanelHeaderProps,
  type DockviewReadyEvent,
} from "dockview-react";
import { themeKandev } from "@/lib/layout/dockview-theme";
import { useDockviewStore, performLayoutSwitch } from "@/lib/state/dockview-store";
import { tryRestoreLayout } from "./dockview-layout-restore";
import {
  setupContainerResizeSync,
  setupGroupTracking,
  setupLayoutPersistence,
  setupPortalCleanup,
} from "./dockview-layout-setup";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useLspFileOpener } from "@/hooks/use-lsp-file-opener";
import { useEditorKeybinds } from "@/hooks/use-editor-keybinds";
import { usePlanPanelAutoOpen } from "@/hooks/use-plan-panel-auto-open";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";

// Panel components (rendered via portals, not directly by dockview)
import { TaskSessionSidebar } from "./task-session-sidebar";
import { LeftHeaderActions, RightHeaderActions } from "./dockview-header-actions";
import { DockviewWatermark } from "./dockview-watermark";
import { TaskChatPanel } from "./task-chat-panel";
import { TaskChangesPanel } from "./task-changes-panel";
import { ChangesPanel } from "./changes-panel";
import { FilesPanel } from "./files-panel";
import { TaskPlanPanel } from "./task-plan-panel";
import { FileEditorPanel } from "./file-editor-panel";
import { PassthroughTerminal } from "./passthrough-terminal";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { ContextMenuTab } from "./tab-context-menu";
import { ChangesTab } from "./changes-tab";
import { PlanTab } from "./plan-tab";
import { PreviewFileTab, PreviewDiffTab, PreviewCommitTab, PinnedDefaultTab } from "./preview-tab";
import { SessionTab } from "./session-tab";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";
import {
  setupSessionTabSync,
  setupChatPanelSafetyNet,
  useAutoSessionTab,
  useAutoPRPanel,
} from "./dockview-session-tabs";
import { TerminalPanel } from "./terminal-panel";
import { BrowserPanel } from "./browser-panel";
import { VscodePanel } from "./vscode-panel";
import { CommitDetailPanel } from "./commit-detail-panel";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { PreviewController } from "./preview/preview-controller";
import { ReviewDialog } from "@/components/review/review-dialog";
import { BottomTerminalPanel } from "./bottom-terminal-panel";
import { useReviewDialog } from "./use-review-dialog";

import type { Repository, RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

// Portal system
import { panelPortalManager, setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { PanelPortalHost, usePortalSlot } from "@/lib/layout/panel-portal-host";

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
const ENV_SCOPED_COMPONENTS = new Set([
  "file-editor",
  "browser",
  "vscode",
  "commit-detail",
  "diff-viewer",
  "pr-detail",
]);

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
const components: Record<string, React.FunctionComponent<IDockviewPanelProps>> = {
  sidebar: PortalSlot,
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
  const onDoubleClick = useTabMaximizeOnDoubleClick(props.api);
  return (
    <div
      className="flex h-full items-center cursor-pointer select-none"
      onDoubleClick={onDoubleClick}
    >
      <DockviewDefaultTab {...props} hideClose />
    </div>
  );
}

/** Sync the user's default saved layout from settings into the dockview store. */
function useSyncUserDefaultLayout() {
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);
  const setUserDefaultLayout = useDockviewStore((s) => s.setUserDefaultLayout);
  useEffect(() => {
    const defaultLayout = savedLayouts.find((l) => l.is_default);
    const state = defaultLayout?.layout as unknown as
      | import("@/lib/state/layout-manager").LayoutState
      | undefined;
    setUserDefaultLayout(state?.columns ? state : null);
  }, [savedLayouts, setUserDefaultLayout]);
}

const tabComponents: Record<string, React.FunctionComponent<IDockviewPanelHeaderProps>> = {
  permanentTab: PermanentTab,
  changesTab: ChangesTab,
  planTab: PlanTab,
  sessionTab: SessionTab,
  previewFileTab: PreviewFileTab,
  previewDiffTab: PreviewDiffTab,
  previewCommitTab: PreviewCommitTab,
  pinnedDefaultTab: PinnedDefaultTab,
};

// ---------------------------------------------------------------------------
// PORTAL CONTENT — the actual panel implementations rendered via portals
// ---------------------------------------------------------------------------

// Each content component renders the real panel UI.  They live permanently
// in the PanelPortalHost and survive dockview layout switches.

function SidebarContent({ panelId }: { panelId: string }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  // Read kanban.workflowId (task snapshot), not workflows.activeId (homepage filter), to preserve "All Workflows" across task navigation.
  const workflowId = useAppStore((state) => state.kanban.workflowId);
  const workspaceName = useAppStore((state) => {
    const ws = state.workspaces.items.find((w: { id: string }) => w.id === workspaceId);
    return ws?.name ?? "Workspace";
  });

  useEffect(() => {
    setPanelTitle(panelId, workspaceName);
  }, [panelId, workspaceName]);

  return <TaskSessionSidebar workspaceId={workspaceId} workflowId={workflowId} />;
}

export const CHAT_PANEL_FALLBACK_LABEL = "Agent";

export function resolveChatPanelTitle(agentLabel: string | null | undefined): string {
  return agentLabel || CHAT_PANEL_FALLBACK_LABEL;
}

function useChatSessionTitle(panelId: string, sessionId: string | null) {
  const agentLabel = useAppStore((state) => {
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    if (!session?.agent_profile_id) return null;
    const profile = state.agentProfiles.items.find(
      (p: { id: string }) => p.id === session.agent_profile_id,
    );
    if (!profile) return null;
    const parts = profile.label.split(" \u2022 ");
    return parts[1] || parts[0] || profile.label;
  });
  useEffect(() => {
    setPanelTitle(panelId, resolveChatPanelTitle(agentLabel));
  }, [panelId, agentLabel]);
}

function ChatContent({ panelId, params }: { panelId: string; params: Record<string, unknown> }) {
  const paramSessionId = params?.sessionId as string | undefined;
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = paramSessionId ?? storeSessionId;
  const { openFile } = useFileEditors();
  const isPassthrough = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.is_passthrough === true : false,
  );
  useChatSessionTitle(panelId, sessionId);

  if (isPassthrough) {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <PassthroughTerminal sessionId={sessionId} mode="agent" />
        </PanelBody>
      </PanelRoot>
    );
  }
  return (
    <TaskChatPanel
      sessionId={sessionId}
      onOpenFile={openFile}
      onOpenFileAtLine={openFile}
      hideSessionsDropdown
    />
  );
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
  const panelKind = (params?.kind as string) ?? "all";
  const selectedPath = panelKind === "file" ? (params?.path as string) : undefined;
  const panelSelectedDiff = panelKind === "all" ? selectedDiff : null;
  const handleClosePanel = useCallback(() => {
    const dockApi = useDockviewStore.getState().api;
    const panel = dockApi?.getPanel(panelId);
    if (dockApi && panel) dockApi.removePanel(panel);
  }, [panelId]);

  return (
    <TaskChangesPanel
      mode={panelKind as "all" | "file"}
      filePath={selectedPath}
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

  const handleEditFile = useCallback((path: string) => openFile(path), [openFile]);
  const handleOpenDiffFile = useCallback(
    (path: string) => addFileDiffPanel(path),
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

function renderPanel(
  panelId: string,
  component: string,
  params: Record<string, unknown>,
): React.ReactNode {
  const resolved = resolveComponent(component);

  switch (resolved) {
    case "sidebar":
      return <SidebarContent panelId={panelId} />;
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
      return <PRDetailPanelComponent panelId={panelId} params={params} />;
    default:
      return <div className="p-4 text-muted-foreground">Unknown panel: {component}</div>;
  }
}

// ---------------------------------------------------------------------------
// LAYOUT RESTORATION HELPERS
// ---------------------------------------------------------------------------

const VALID_COMPONENTS = new Set(Object.keys(components));

// ---------------------------------------------------------------------------
// useEnvSwitchCleanup — backup layout switch for external session changes
// ---------------------------------------------------------------------------

function useEnvSwitchCleanup(effectiveSessionId: string | null, effectiveEnvId: string | null) {
  const prevEnvRef = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    const newEnvId = effectiveEnvId;
    if (prevEnvRef.current === undefined) {
      prevEnvRef.current = newEnvId;
      return;
    }
    if (prevEnvRef.current === newEnvId) return;

    const oldEnvId = prevEnvRef.current;
    prevEnvRef.current = newEnvId;

    // Portal cleanup is handled synchronously inside switchEnvLayout (in the
    // dockview store action) before any fromJSON call. This hook serves as a
    // backup for external session changes (e.g. WS-driven) that don't go
    // through the sidebar/dropdown switch helpers. Same-env switches return
    // early above (no-op).
    if (newEnvId) {
      performLayoutSwitch(oldEnvId, newEnvId, effectiveSessionId);
    }
  }, [effectiveEnvId, effectiveSessionId]);
}

// ---------------------------------------------------------------------------
// MAIN LAYOUT COMPONENT
// ---------------------------------------------------------------------------

type DockviewDesktopLayoutProps = {
  workspaceId: string | null;
  workflowId: string | null;
  sessionId?: string | null;
  repository?: Repository | null;
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
  initialLayout?: string | null;
};

export const DockviewDesktopLayout = memo(function DockviewDesktopLayout({
  sessionId,
  repository,
  initialLayout,
}: DockviewDesktopLayoutProps) {
  const setApi = useDockviewStore((s) => s.setApi);
  const buildDefaultLayout = useDockviewStore((s) => s.buildDefaultLayout);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const readyDisposersRef = useRef<Array<() => void>>([]);
  const appStore = useAppStoreApi();

  const effectiveSessionId =
    useAppStore((state) => state.tasks.activeSessionId) ?? sessionId ?? null;
  const effectiveEnvId = useAppStore((state) =>
    effectiveSessionId ? (state.environmentIdBySessionId[effectiveSessionId] ?? null) : null,
  );
  const envIdRef = useRef<string | null>(effectiveEnvId);
  const hasDevScript = Boolean(repository?.dev_script?.trim());

  const review = useReviewDialog(effectiveSessionId);

  useSyncUserDefaultLayout();
  useLspFileOpener();
  useEditorKeybinds();
  usePlanPanelAutoOpen();

  useEffect(() => {
    envIdRef.current = effectiveEnvId;
  }, [effectiveEnvId]);

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      const api = event.api;
      setApi(api);

      const currentEnvId = envIdRef.current;
      // If a layout intent was passed via URL, skip saved layout restoration
      const restored = !initialLayout && tryRestoreLayout(api, currentEnvId, VALID_COMPONENTS);
      if (!restored) {
        buildDefaultLayout(api, initialLayout ?? undefined);
      }

      useDockviewStore.setState({ currentLayoutEnvId: currentEnvId });

      readyDisposersRef.current.push(setupGroupTracking(api));
      setupSessionTabSync(api, appStore);
      setupChatPanelSafetyNet(api, appStore);
      setupLayoutPersistence(api, saveTimerRef, envIdRef);
      setupPortalCleanup(api, appStore);
      readyDisposersRef.current.push(setupContainerResizeSync(api));
    },
    [setApi, buildDefaultLayout, initialLayout, appStore],
  );

  // Release session-scoped portals + trigger layout switch on session change.
  // IMPORTANT: this must run BEFORE useAutoSessionTab so the old layout is
  // saved before a new session tab is created — otherwise the new session's
  // panel could leak into the old session's persisted layout.
  useEnvSwitchCleanup(effectiveSessionId, effectiveEnvId);

  // Auto-create a session tab when a session becomes active
  useAutoSessionTab(effectiveSessionId);

  // Auto-show PR detail panel when the task has an associated PR
  useAutoPRPanel();

  // Clean up on unmount (e.g. navigating away from session page)
  useEffect(() => {
    const timerRef = saveTimerRef;
    const disposersRef = readyDisposersRef;
    return () => {
      for (const dispose of disposersRef.current.splice(0)) dispose();
      if (timerRef.current) clearTimeout(timerRef.current);
      panelPortalManager.releaseAll();
    };
  }, []);

  // Visual masking: hide the dockview container during slow-path layout
  // switches (full fromJSON rebuild) to prevent the old layout from flashing.
  const isRestoringLayout = useDockviewStore((s) => s.isRestoringLayout);

  return (
    <div
      className="flex-1 min-h-0 grid grid-rows-[1fr_auto]"
      aria-busy={isRestoringLayout}
      style={{
        opacity: isRestoringLayout ? 0 : 1,
        pointerEvents: isRestoringLayout ? "none" : undefined,
        transition: isRestoringLayout ? "none" : "opacity 60ms ease-out",
      }}
    >
      <div className="min-h-0 min-w-0 overflow-hidden flex flex-col">
        <PreviewController sessionId={effectiveSessionId} hasDevScript={hasDevScript} />
        <DockviewReact
          theme={themeKandev}
          components={components}
          tabComponents={tabComponents}
          defaultTabComponent={ContextMenuTab}
          leftHeaderActionsComponent={LeftHeaderActions}
          rightHeaderActionsComponent={RightHeaderActions}
          watermarkComponent={DockviewWatermark}
          onReady={onReady}
          defaultRenderer="always"
          className="flex-1 min-h-0"
        />
      </div>
      <BottomTerminalPanel />
      <PanelPortalHost renderPanel={renderPanel} />
      {effectiveSessionId && (
        <ReviewDialog
          open={review.reviewDialogOpen}
          onOpenChange={review.setReviewDialogOpen}
          sessionId={effectiveSessionId}
          baseBranch={review.baseBranch}
          onSendComments={review.handleReviewSendComments}
          onOpenFile={review.reviewOpenFile}
          gitStatusFiles={review.reviewGitStatusFiles}
          cumulativeDiff={review.reviewCumulativeDiff}
          prDiffFiles={review.reviewPRDiffFiles}
        />
      )}
    </div>
  );
});
