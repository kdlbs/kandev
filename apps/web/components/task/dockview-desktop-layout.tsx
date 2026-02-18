'use client';

import { useCallback, useEffect, useRef, useState, memo } from 'react';
import {
  DockviewReact,
  DockviewDefaultTab,
  type IDockviewPanelProps,
  type IDockviewPanelHeaderProps,
  type DockviewReadyEvent,
  type SerializedDockview,
} from 'dockview-react';
import { themeKandev } from '@/lib/layout/dockview-theme';
import {
  useDockviewStore,
  applyLayoutFixups,
  performLayoutSwitch,
  LAYOUT_SIDEBAR_RATIO,
  LAYOUT_SIDEBAR_MAX_PX,
  LAYOUT_RIGHT_RATIO,
  LAYOUT_RIGHT_MAX_PX,
} from '@/lib/state/dockview-store';
import { getSessionLayout, setSessionLayout } from '@/lib/local-storage';
import { useAppStore } from '@/components/state-provider';
import { useFileEditors } from '@/hooks/use-file-editors';
import { useLspFileOpener } from '@/hooks/use-lsp-file-opener';
import { useEditorKeybinds } from '@/hooks/use-editor-keybinds';
import { useSessionGitStatus } from '@/hooks/domains/session/use-session-git-status';
import { useSessionCommits } from '@/hooks/domains/session/use-session-commits';

// Panel components
import { TaskSessionSidebar } from './task-session-sidebar';
import { LeftHeaderActions, RightHeaderActions } from './dockview-header-actions';
import { TaskChatPanel } from './task-chat-panel';
import { TaskChangesPanel } from './task-changes-panel';
import { ChangesPanel } from './changes-panel';
import { FilesPanel } from './files-panel';
import { TaskPlanPanel } from './task-plan-panel';
import { FileEditorPanel } from './file-editor-panel';
import { PassthroughTerminal } from './passthrough-terminal';
import { PanelRoot, PanelBody } from './panel-primitives';
import { TerminalPanel } from './terminal-panel';
import { BrowserPanel } from './browser-panel';
import { CommitDetailPanel } from './commit-detail-panel';
import { PreviewController } from './preview/preview-controller';
import { ReviewDialog } from '@/components/review/review-dialog';
import { useCumulativeDiff } from '@/hooks/domains/session/use-cumulative-diff';
import { formatReviewCommentsAsMarkdown } from '@/components/task/chat/messages/review-comments-attachment';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useToast } from '@/components/toast-provider';
import type { DiffComment } from '@/lib/diff/types';

import type { Repository, RepositoryScript } from '@/lib/types/http';
import type { Terminal } from '@/hooks/domains/session/use-terminals';

// --- STORAGE KEY ---
const LAYOUT_STORAGE_KEY = 'dockview-layout-v1';

// --- PANEL COMPONENTS ---
// Each panel is a standalone component wrapped for dockview

function SidebarPanel(props: IDockviewPanelProps) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const workspaceName = useAppStore((state) => {
    const ws = state.workspaces.items.find((w: { id: string }) => w.id === workspaceId);
    return ws?.name ?? 'Workspace';
  });

  // Keep the dockview tab title in sync with workspace name
  useEffect(() => {
    if (props.api.title !== workspaceName) {
      props.api.setTitle(workspaceName);
    }
  }, [props.api, workspaceName]);

  return <TaskSessionSidebar workspaceId={workspaceId} workflowId={workflowId} />;
}

function ChatPanel(props: IDockviewPanelProps) {
  const groupId = props.api.group.id;
  const isPanelFocused = useDockviewStore((s) => s.activeGroupId === groupId);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { openFile } = useFileEditors();

  const isPassthrough = useAppStore((state) => {
    if (!sessionId) return false;
    return state.taskSessions.items[sessionId]?.is_passthrough === true;
  });

  useEffect(() => {
    props.api.setTitle('Agent');
  }, [props.api]);

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
      isPanelFocused={isPanelFocused}
    />
  );
}

function DiffViewerPanelComponent() {
  const selectedDiff = useDockviewStore((s) => s.selectedDiff);
  const setSelectedDiff = useDockviewStore((s) => s.setSelectedDiff);
  const { openFile } = useFileEditors();

  return (
    <TaskChangesPanel
      selectedDiff={selectedDiff}
      onClearSelected={() => setSelectedDiff(null)}
      onOpenFile={openFile}
    />
  );
}

function ChangesPanelWrapper(props: IDockviewPanelProps) {
  const addDiffViewerPanel = useDockviewStore((s) => s.addDiffViewerPanel);
  const addCommitDetailPanel = useDockviewStore((s) => s.addCommitDetailPanel);
  const { openFile } = useFileEditors();

  // Dynamic title with file count
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);

  const fileCount = gitStatus?.files ? Object.keys(gitStatus.files).length : 0;
  const totalCount = fileCount + commits.length;

  useEffect(() => {
    const title = totalCount > 0 ? `Changes (${totalCount})` : 'Changes';
    if (props.api.title !== title) {
      props.api.setTitle(title);
    }
  }, [totalCount, props.api]);

  const handleOpenFile = useCallback((path: string) => {
    openFile(path);
  }, [openFile]);

  const handleOpenCommitDetail = useCallback((sha: string) => {
    addCommitDetailPanel(sha);
  }, [addCommitDetailPanel]);

  const handleOpenDiffAll = useCallback(() => {
    addDiffViewerPanel();
  }, [addDiffViewerPanel]);

  const handleOpenReview = useCallback(() => {
    window.dispatchEvent(new CustomEvent('open-review-dialog'));
  }, []);

  return (
    <ChangesPanel
      onOpenFile={handleOpenFile}
      onOpenCommitDetail={handleOpenCommitDetail}
      onOpenDiffAll={handleOpenDiffAll}
      onOpenReview={handleOpenReview}
    />
  );
}

function FilesPanelWrapper() {
  const { openFile } = useFileEditors();

  const handleOpenFile = useCallback(
    (file: { path: string; name: string; content: string; originalContent?: string; originalHash?: string; isDirty?: boolean; isBinary?: boolean }) => {
      openFile(file.path);
    },
    [openFile]
  );

  return <FilesPanel onOpenFile={handleOpenFile} />;
}

function PlanPanelComponent() {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  return <TaskPlanPanel taskId={taskId} visible />;
}

// --- COMPONENT MAP ---
const components: Record<string, React.FunctionComponent<IDockviewPanelProps>> = {
  sidebar: SidebarPanel,
  chat: ChatPanel,
  'diff-viewer': DiffViewerPanelComponent,
  'file-editor': FileEditorPanel,
  'commit-detail': CommitDetailPanel,
  changes: ChangesPanelWrapper,
  files: FilesPanelWrapper,
  terminal: TerminalPanel,
  browser: BrowserPanel,
  plan: PlanPanelComponent,
  // Backwards compat aliases for saved layouts
  'diff-files': ChangesPanelWrapper,
  'all-files': FilesPanelWrapper,
};

// --- TAB COMPONENTS ---
// Permanent tab — same as default but without close button
function PermanentTab(props: IDockviewPanelHeaderProps) {
  return <DockviewDefaultTab {...props} hideClose />;
}

const tabComponents: Record<string, React.FunctionComponent<IDockviewPanelHeaderProps>> = {
  permanentTab: PermanentTab,
};

// --- LAYOUT RESTORATION HELPERS ---

/** Try to restore layout from per-session or global localStorage. Returns true if restored. */
function tryRestoreLayout(
  api: DockviewReadyEvent['api'],
  currentSessionId: string | null,
): boolean {
  // 1. Try per-session layout
  if (currentSessionId) {
    try {
      const sessionLayout = getSessionLayout(currentSessionId);
      if (sessionLayout) {
        api.fromJSON(sessionLayout as SerializedDockview);
        useDockviewStore.setState(applyLayoutFixups(api));
        return true;
      }
    } catch {
      // Per-session restore failed, try global
    }
  }

  // 2. Fallback to global localStorage — only when there's no session context
  //    (true first load). If we have a session ID but no saved layout, it's a
  //    new session and should get the default layout, not an old global one.
  if (!currentSessionId) {
    try {
      const saved = localStorage.getItem(LAYOUT_STORAGE_KEY);
      if (saved) {
        const layout = JSON.parse(saved);
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

/** Enforce column max widths after group add/remove (dockview redistributes all columns equally). */
function enforceColumnMaxWidths(api: DockviewReadyEvent['api']) {
  requestAnimationFrame(() => {
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const rootSplitview = (api as any).component?.gridview?.root?.splitview;
      if (!rootSplitview?.resizeView || !rootSplitview?.getViewSize) return;
      const count = rootSplitview.length;
      if (count < 3) return;

      const sidebarMax = Math.min(Math.round(api.width * LAYOUT_SIDEBAR_RATIO), LAYOUT_SIDEBAR_MAX_PX);
      const rightMax = Math.min(Math.round(api.width * LAYOUT_RIGHT_RATIO), LAYOUT_RIGHT_MAX_PX);

      if (rootSplitview.getViewSize(0) > sidebarMax) {
        rootSplitview.resizeView(0, sidebarMax);
      }
      if (rootSplitview.getViewSize(count - 1) > rightMax) {
        rootSplitview.resizeView(count - 1, rightMax);
      }

      const centerCount = count - 2;
      if (centerCount > 1) {
        const centerTotal = api.width - rootSplitview.getViewSize(0) - rootSplitview.getViewSize(count - 1);
        const equalSize = Math.round(centerTotal / centerCount);
        for (let i = 1; i <= centerCount; i++) {
          rootSplitview.resizeView(i, equalSize);
        }
      }
    } catch {
      // Internal API may change between versions
    }
  });
}

/** Re-add the chat panel if it gets removed (keeps center group alive). */
function setupChatPanelSafetyNet(api: DockviewReadyEvent['api']) {
  api.onDidRemovePanel((panel) => {
    if (panel.id !== 'chat') return;
    // Skip during layout restore — fromJSON removes all panels before re-adding
    if (useDockviewStore.getState().isRestoringLayout) return;
    requestAnimationFrame(() => {
      if (api.getPanel('chat')) return;
      const sidebarPanel = api.getPanel('sidebar');
      api.addPanel({
        id: 'chat',
        component: 'chat',
        tabComponent: 'permanentTab',
        title: 'Agent',
        position: sidebarPanel
          ? { direction: 'right', referencePanel: 'sidebar' }
          : undefined,
      });
      const newChat = api.getPanel('chat');
      if (newChat) {
        useDockviewStore.setState({ centerGroupId: newChat.group.id });
      }
    });
  });
}

/** Debounced layout persistence on every layout change. */
function setupLayoutPersistence(
  api: DockviewReadyEvent['api'],
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

/** Hook encapsulating the review dialog state and send-comments handler. */
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

  const handleReviewSendComments = useCallback(
    (comments: DiffComment[]) => {
      if (!activeTaskId || !effectiveSessionId || comments.length === 0) return;
      const client = getWebSocketClient();
      if (!client) return;
      const markdown = formatReviewCommentsAsMarkdown(comments);
      client.request('message.add', {
        task_id: activeTaskId,
        session_id: effectiveSessionId,
        content: markdown,
      }, 10000).catch(() => {
        toast({ title: 'Failed to send comments', variant: 'error' });
      });
      setReviewDialogOpen(false);
    },
    [activeTaskId, effectiveSessionId, toast]
  );

  // Listen for open-review-dialog events from any panel
  useEffect(() => {
    const handler = () => setReviewDialogOpen(true);
    window.addEventListener('open-review-dialog', handler);
    return () => window.removeEventListener('open-review-dialog', handler);
  }, []);

  return {
    reviewDialogOpen,
    setReviewDialogOpen,
    baseBranch,
    reviewGitStatus,
    reviewCumulativeDiff,
    reviewOpenFile,
    handleReviewSendComments,
  };
}

// --- MAIN LAYOUT COMPONENT ---
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

  const effectiveSessionId = useAppStore((state) => state.tasks.activeSessionId) ?? sessionId ?? null;
  const hasDevScript = Boolean(repository?.dev_script?.trim());

  const review = useReviewDialog(effectiveSessionId);

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

      api.onDidAddGroup(() => enforceColumnMaxWidths(api));
      api.onDidRemoveGroup(() => enforceColumnMaxWidths(api));
      setupChatPanelSafetyNet(api);
      setupLayoutPersistence(api, saveTimerRef, sessionIdRef);
    },
    [setApi, buildDefaultLayout]
  );

  // Catch-all: detect session changes and trigger layout switch
  const prevSessionRef = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    if (prevSessionRef.current === undefined) {
      prevSessionRef.current = effectiveSessionId;
      return;
    }
    if (prevSessionRef.current === effectiveSessionId) return;

    const oldSessionId = prevSessionRef.current;
    prevSessionRef.current = effectiveSessionId;

    if (effectiveSessionId) {
      performLayoutSwitch(oldSessionId, effectiveSessionId);
    }
  }, [effectiveSessionId]);

  // Clean up timer on unmount
  useEffect(() => {
    const timerRef = saveTimerRef;
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  return (
    <div className="flex-1 min-h-0">
      <PreviewController sessionId={effectiveSessionId} hasDevScript={hasDevScript} />
      <DockviewReact
        theme={themeKandev}
        components={components}
        tabComponents={tabComponents}
        leftHeaderActionsComponent={LeftHeaderActions}
        rightHeaderActionsComponent={RightHeaderActions}
        onReady={onReady}
        defaultRenderer="always"
        className="h-full"
      />
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
        />
      )}
    </div>
  );
});
