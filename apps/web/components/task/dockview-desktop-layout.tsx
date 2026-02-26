"use client";

import React, { useCallback, useEffect, useRef, useState, memo } from "react";
import {
  DockviewReact,
  DockviewDefaultTab,
  type IDockviewPanelProps,
  type IDockviewPanelHeaderProps,
  type DockviewReadyEvent,
  type SerializedDockview,
} from "dockview-react";
import { themeKandev } from "@/lib/layout/dockview-theme";
import { useDockviewStore, performLayoutSwitch } from "@/lib/state/dockview-store";
import { applyLayoutFixups, getRootSplitview } from "@/lib/state/dockview-layout-builders";
import { getSessionLayout, setSessionLayout } from "@/lib/local-storage";
import { useAppStore } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useLspFileOpener } from "@/hooks/use-lsp-file-opener";
import { useEditorKeybinds } from "@/hooks/use-editor-keybinds";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";

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
import { TerminalPanel } from "./terminal-panel";
import { BrowserPanel } from "./browser-panel";
import { VscodePanel } from "./vscode-panel";
import { CommitDetailPanel } from "./commit-detail-panel";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { PreviewController } from "./preview/preview-controller";
import { ReviewDialog } from "@/components/review/review-dialog";
import { useCumulativeDiff } from "@/hooks/domains/session/use-cumulative-diff";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { formatReviewCommentsAsMarkdown } from "@/components/task/chat/messages/review-comments-attachment";
import { stopVscode } from "@/lib/api/domains/vscode-api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useToast } from "@/components/toast-provider";
import type { DiffComment } from "@/lib/diff/types";

import type { Repository, RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";

// Portal system
import { panelPortalManager, setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { PanelPortalHost, usePortalSlot } from "@/lib/layout/panel-portal-host";

// --- STORAGE KEY ---
const LAYOUT_STORAGE_KEY = "dockview-layout-v1";

// ---------------------------------------------------------------------------
// PORTAL SLOT — generic dockview component that adopts a persistent portal
// ---------------------------------------------------------------------------

/**
 * Components whose portals are tied to a specific session.
 *
 * When the user switches sessions, portals for these components are released
 * via `panelPortalManager.releaseBySession()` so stale state (WebSocket
 * connections, iframes, editor buffers) from the old session doesn't leak
 * into the new one.
 *
 * A component belongs here if its content is bound to session-specific runtime
 * state that can't be swapped by simply reading a new `activeSessionId` from
 * the store:
 *
 *  - terminal      — holds a live xterm WebSocket to a shell in the session's container
 *  - file-editor   — editing a file in the session's worktree
 *  - browser       — iframe preview of the session's dev server URL
 *  - vscode        — VS Code Server iframe running in the session's container
 *  - commit-detail — displays a commit from the session's git history
 *  - diff-viewer   — shows file diffs from the session's working tree
 *  - pr-detail     — PR linked to the session's task
 *
 * Components NOT listed here are **global** — they read `activeSessionId`
 * reactively from the store and automatically reflect the current session:
 *
 *  - sidebar  — workspace/task navigation, not session-specific
 *  - chat     — subscribes to `activeSessionId`, re-renders for new session
 *  - changes  — reads session git status via `useSessionGitStatus(activeSessionId)`
 *  - files    — reads the active session's file tree reactively
 *  - plan     — reads `activeTaskId` from the store
 */
const SESSION_SCOPED_COMPONENTS = new Set([
  "terminal",
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
 * Session-scoped panels are tagged with the current session ID so they can
 * be cleaned up on session switch.
 */
function PortalSlot(props: IDockviewPanelProps) {
  const component = props.api.component;
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const sessionId = SESSION_SCOPED_COMPONENTS.has(component)
    ? (activeSessionId ?? undefined)
    : undefined;
  const containerRef = usePortalSlot(props, sessionId);
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
  return <DockviewDefaultTab {...props} hideClose />;
}

const tabComponents: Record<string, React.FunctionComponent<IDockviewPanelHeaderProps>> = {
  permanentTab: PermanentTab,
};

// ---------------------------------------------------------------------------
// PORTAL CONTENT — the actual panel implementations rendered via portals
// ---------------------------------------------------------------------------

// Each content component renders the real panel UI.  They live permanently
// in the PanelPortalHost and survive dockview layout switches.

function SidebarContent({ panelId }: { panelId: string }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const workspaceName = useAppStore((state) => {
    const ws = state.workspaces.items.find((w: { id: string }) => w.id === workspaceId);
    return ws?.name ?? "Workspace";
  });

  useEffect(() => {
    setPanelTitle(panelId, workspaceName);
  }, [panelId, workspaceName]);

  return <TaskSessionSidebar workspaceId={workspaceId} workflowId={workflowId} />;
}

function ChatContent({ panelId }: { panelId: string }) {
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { openFile } = useFileEditors();

  const isPassthrough = useAppStore((state) => {
    if (!sessionId) return false;
    return state.taskSessions.items[sessionId]?.is_passthrough === true;
  });

  useEffect(() => {
    setPanelTitle(panelId, "Agent");
  }, [panelId]);

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
      isPanelFocused={false}
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

  // Dynamic title with file count
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
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
    (sha: string) => addCommitDetailPanel(sha),
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
      return <ChatContent panelId={panelId} />;
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

// ---------------------------------------------------------------------------
// LAYOUT RESTORATION HELPERS
// ---------------------------------------------------------------------------

const VALID_COMPONENTS = new Set(Object.keys(components));

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function sanitizeLayout(layout: any): any {
  if (!layout?.panels || !layout?.grid?.root) return layout;

  const invalidIds = new Set<string>();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const validPanels: Record<string, any> = {};
  for (const [id, panel] of Object.entries(layout.panels)) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const comp = (panel as any).contentComponent;
    if (comp && VALID_COMPONENTS.has(comp)) {
      validPanels[id] = panel;
    } else {
      invalidIds.add(id);
    }
  }

  if (invalidIds.size === 0) return layout;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function cleanNode(node: any): any {
    if (node.type === "leaf") {
      const views = (node.data.views as string[]).filter((v) => !invalidIds.has(v));
      if (views.length === 0) return null;
      const activeView = views.includes(node.data.activeView) ? node.data.activeView : views[0];
      return { ...node, data: { ...node.data, views, activeView } };
    }
    if (node.type === "branch") {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const children = (node.data as any[]).map(cleanNode).filter(Boolean);
      if (children.length === 0) return null;
      return { ...node, data: children };
    }
    return node;
  }

  const cleanedRoot = cleanNode(layout.grid.root);
  if (!cleanedRoot) return null;

  return {
    ...layout,
    grid: { ...layout.grid, root: cleanedRoot },
    panels: validPanels,
  };
}

function tryRestoreLayout(
  api: DockviewReadyEvent["api"],
  currentSessionId: string | null,
): boolean {
  if (currentSessionId) {
    try {
      const sessionLayout = getSessionLayout(currentSessionId);
      if (sessionLayout) {
        const sanitized = sanitizeLayout(sessionLayout);
        if (!sanitized) return false;
        api.fromJSON(sanitized as SerializedDockview);
        useDockviewStore.setState(applyLayoutFixups(api));
        return true;
      }
    } catch {
      // Per-session restore failed, try global
    }
  }

  if (!currentSessionId) {
    try {
      const saved = localStorage.getItem(LAYOUT_STORAGE_KEY);
      if (saved) {
        const layout = sanitizeLayout(JSON.parse(saved));
        if (!layout) return false;
        api.fromJSON(layout);
        useDockviewStore.setState(applyLayoutFixups(api));
        return true;
      }
    } catch {
      // Global restore failed, build default
    }
  }

  return false;
}

function trackPinnedWidths(api: DockviewReadyEvent["api"]): void {
  if (useDockviewStore.getState().isRestoringLayout) return;
  const sv = getRootSplitview(api);
  if (!sv || sv.length < 2) return;
  try {
    const sidebarW = sv.getViewSize(0);
    if (sidebarW > 50) {
      const current = useDockviewStore.getState().pinnedWidths.get("sidebar");
      if (current !== sidebarW) {
        useDockviewStore.getState().setPinnedWidth("sidebar", sidebarW);
      }
    }
    if (sv.length >= 3) {
      const rightIdx = sv.length - 1;
      const rightW = sv.getViewSize(rightIdx);
      if (rightW > 50) {
        const current = useDockviewStore.getState().pinnedWidths.get("right");
        if (current !== rightW) {
          useDockviewStore.getState().setPinnedWidth("right", rightW);
        }
      }
    }
  } catch {
    /* noop */
  }
}

function setupChatPanelSafetyNet(api: DockviewReadyEvent["api"]) {
  api.onDidRemovePanel((panel) => {
    if (panel.id !== "chat") return;
    if (useDockviewStore.getState().isRestoringLayout) return;
    requestAnimationFrame(() => {
      if (api.getPanel("chat")) return;
      const sidebarPanel = api.getPanel("sidebar");
      api.addPanel({
        id: "chat",
        component: "chat",
        tabComponent: "permanentTab",
        title: "Agent",
        position: sidebarPanel ? { direction: "right", referencePanel: "sidebar" } : undefined,
      });
      const newChat = api.getPanel("chat");
      if (newChat) {
        useDockviewStore.setState({ centerGroupId: newChat.group.id });
      }
    });
  });
}

function setupLayoutPersistence(
  api: DockviewReadyEvent["api"],
  saveTimerRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>,
  sessionIdRef: React.MutableRefObject<string | null>,
) {
  api.onDidLayoutChange(() => {
    if (useDockviewStore.getState().isRestoringLayout) return;

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      try {
        const json = api.toJSON();
        localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(json));
        const sid = sessionIdRef.current;
        if (sid) {
          setSessionLayout(sid, json);
        }
      } catch {
        // Ignore serialization errors
      }
    }, 300);
  });
}

/**
 * Clean up portal entries for panels that were permanently removed (user
 * closed a tab), but NOT during layout restores where fromJSON temporarily
 * removes all panels.
 */
function setupPortalCleanup(api: DockviewReadyEvent["api"]) {
  api.onDidRemovePanel((panel) => {
    if (useDockviewStore.getState().isRestoringLayout) return;
    // Stop code-server when the user explicitly closes the vscode tab
    const entry = panelPortalManager.get(panel.id);
    if (entry?.component === "vscode" && entry.sessionId) {
      stopVscode(entry.sessionId);
    }
    panelPortalManager.release(panel.id);
  });
}

// ---------------------------------------------------------------------------
// useReviewDialog hook
// ---------------------------------------------------------------------------

function useReviewDialog(effectiveSessionId: string | null) {
  const [reviewDialogOpen, setReviewDialogOpen] = useState(false);
  const { toast } = useToast();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const baseBranch = useAppStore((state) => {
    if (!effectiveSessionId) return undefined;
    return state.taskSessions.items[effectiveSessionId]?.base_branch;
  });
  const reviewGitStatus = useSessionGitStatus(effectiveSessionId);
  const { diff: reviewCumulativeDiff } = useCumulativeDiff(effectiveSessionId);
  const { openFile: reviewOpenFile } = useFileEditors();
  const reviewTaskPR = useActiveTaskPR();
  const { files: reviewPRDiffFiles } = usePRDiff(
    reviewTaskPR?.owner ?? null,
    reviewTaskPR?.repo ?? null,
    reviewTaskPR?.pr_number ?? null,
  );

  const handleReviewSendComments = useCallback(
    (comments: DiffComment[]) => {
      if (!activeTaskId || !effectiveSessionId || comments.length === 0) return;
      const client = getWebSocketClient();
      if (!client) return;
      const markdown = formatReviewCommentsAsMarkdown(comments);
      client
        .request(
          "message.add",
          {
            task_id: activeTaskId,
            session_id: effectiveSessionId,
            content: markdown,
          },
          10000,
        )
        .catch(() => {
          toast({ title: "Failed to send comments", variant: "error" });
        });
      setReviewDialogOpen(false);
    },
    [activeTaskId, effectiveSessionId, toast],
  );

  useEffect(() => {
    const handler = () => setReviewDialogOpen(true);
    window.addEventListener("open-review-dialog", handler);
    return () => window.removeEventListener("open-review-dialog", handler);
  }, []);

  return {
    reviewDialogOpen,
    setReviewDialogOpen,
    baseBranch,
    reviewGitStatus,
    reviewCumulativeDiff,
    reviewPRDiffFiles,
    reviewOpenFile,
    handleReviewSendComments,
  };
}

// ---------------------------------------------------------------------------
// useSessionSwitchCleanup — releases session-scoped portals + triggers layout switch
// ---------------------------------------------------------------------------

function useSessionSwitchCleanup(effectiveSessionId: string | null) {
  const prevSessionRef = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    if (prevSessionRef.current === undefined) {
      prevSessionRef.current = effectiveSessionId;
      return;
    }
    if (prevSessionRef.current === effectiveSessionId) return;

    const oldSessionId = prevSessionRef.current;
    prevSessionRef.current = effectiveSessionId;

    // Release session-scoped portals from the old session so stale
    // terminals, editors, etc. don't leak into the new session.
    if (oldSessionId) {
      panelPortalManager.releaseBySession(oldSessionId);
    }

    if (effectiveSessionId) {
      performLayoutSwitch(oldSessionId, effectiveSessionId);
    }
  }, [effectiveSessionId]);
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
};

export const DockviewDesktopLayout = memo(function DockviewDesktopLayout({
  sessionId,
  repository,
}: DockviewDesktopLayoutProps) {
  const setApi = useDockviewStore((s) => s.setApi);
  const buildDefaultLayout = useDockviewStore((s) => s.buildDefaultLayout);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sessionIdRef = useRef<string | null>(null);

  const effectiveSessionId =
    useAppStore((state) => state.tasks.activeSessionId) ?? sessionId ?? null;
  const hasDevScript = Boolean(repository?.dev_script?.trim());

  const review = useReviewDialog(effectiveSessionId);

  // Sync user's default saved layout into dockview store
  const savedLayouts = useAppStore((s) => s.userSettings.savedLayouts);
  const setUserDefaultLayout = useDockviewStore((s) => s.setUserDefaultLayout);
  useEffect(() => {
    const defaultLayout = savedLayouts.find((l) => l.is_default);
    const state = defaultLayout?.layout as unknown as
      | import("@/lib/state/layout-manager").LayoutState
      | undefined;
    setUserDefaultLayout(state?.columns ? state : null);
  }, [savedLayouts, setUserDefaultLayout]);

  // Connect LSP Go-to-Definition navigation to dockview file tabs
  useLspFileOpener();

  // Global editor keybinds (tab nav, terminal toggle)
  useEditorKeybinds();

  // Keep sessionIdRef in sync for use inside event handlers
  useEffect(() => {
    sessionIdRef.current = effectiveSessionId;
  }, [effectiveSessionId]);

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      const api = event.api;
      setApi(api);

      const currentSessionId = sessionIdRef.current;
      const restored = tryRestoreLayout(api, currentSessionId);
      if (!restored) {
        buildDefaultLayout(api);
      }

      useDockviewStore.setState({ currentLayoutSessionId: currentSessionId });

      // Track active group
      api.onDidActiveGroupChange((group) => {
        useDockviewStore.setState({ activeGroupId: group?.id ?? null });
      });
      useDockviewStore.setState({ activeGroupId: api.activeGroup?.id ?? null });

      // Track pinned column widths on layout changes (captures user sash drags)
      api.onDidLayoutChange(() => trackPinnedWidths(api));
      trackPinnedWidths(api);

      setupChatPanelSafetyNet(api);
      setupLayoutPersistence(api, saveTimerRef, sessionIdRef);
      setupPortalCleanup(api);
    },
    [setApi, buildDefaultLayout],
  );

  // Release session-scoped portals + trigger layout switch on session change
  useSessionSwitchCleanup(effectiveSessionId);

  // Clean up on unmount (e.g. navigating away from session page)
  useEffect(() => {
    const timerRef = saveTimerRef;
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
      panelPortalManager.releaseAll();
    };
  }, []);

  return (
    <div className="flex-1 min-h-0">
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
        className="h-full"
      />
      <PanelPortalHost renderPanel={renderPanel} />
      {effectiveSessionId && (
        <ReviewDialog
          open={review.reviewDialogOpen}
          onOpenChange={review.setReviewDialogOpen}
          sessionId={effectiveSessionId}
          baseBranch={review.baseBranch}
          onSendComments={review.handleReviewSendComments}
          onOpenFile={review.reviewOpenFile}
          gitStatusFiles={review.reviewGitStatus?.files ?? null}
          cumulativeDiff={review.reviewCumulativeDiff}
          prDiffFiles={review.reviewPRDiffFiles}
        />
      )}
    </div>
  );
});
