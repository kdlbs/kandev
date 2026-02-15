'use client';

import { useCallback, useEffect, useRef, memo } from 'react';
import {
  DockviewReact,
  DockviewDefaultTab,
  type IDockviewPanelProps,
  type IDockviewPanelHeaderProps,
  type IDockviewHeaderActionsProps,
  type DockviewReadyEvent,
  type SerializedDockview,
} from 'dockview-react';
import {
  IconPlus,
  IconDeviceDesktop,
  IconTerminal2,
  IconFileText,
  IconFolder,
  IconGitBranch,
  IconPlayerPlay,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
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
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { useFileEditors } from '@/hooks/use-file-editors';
import { startProcess } from '@/lib/api';
import { createUserShell } from '@/lib/api/domains/user-shell-api';
import { useRepositoryScripts } from '@/hooks/domains/workspace/use-repository-scripts';
import { useLspFileOpener } from '@/hooks/use-lsp-file-opener';

// Panel components
import { TaskSessionSidebar, NewTaskButton } from './task-session-sidebar';
import { TaskChatPanel } from './task-chat-panel';
import { TaskChangesPanel } from './task-changes-panel';
import { ChangesPanel } from './changes-panel';
import { FilesPanel } from './files-panel';
import { TaskPlanPanel } from './task-plan-panel';
import { FileEditorPanel } from './file-editor-panel';
import { TerminalPanel } from './terminal-panel';
import { BrowserPanel } from './browser-panel';
import { PreviewController } from './preview/preview-controller';

import type { Repository, RepositoryScript, Task, ProcessInfo } from '@/lib/types/http';
import type { ProcessStatusEntry } from '@/lib/state/slices';
import { linkToSession } from '@/lib/links';
import type { Terminal } from '@/hooks/domains/session/use-terminals';

/** Map a ProcessInfo response to a ProcessStatusEntry for the store. */
function mapProcessToStatus(process: ProcessInfo): ProcessStatusEntry {
  return {
    processId: process.id,
    sessionId: process.session_id,
    kind: process.kind,
    scriptName: process.script_name,
    status: process.status,
    command: process.command,
    workingDir: process.working_dir,
    exitCode: process.exit_code ?? null,
    startedAt: process.started_at,
    updatedAt: process.updated_at,
  };
}

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

function ChangesPanelWrapper() {
  const addDiffViewerPanel = useDockviewStore((s) => s.addDiffViewerPanel);

  const handleSelectDiff = useCallback((path: string, content?: string) => {
    addDiffViewerPanel(path, content);
  }, [addDiffViewerPanel]);

  return <ChangesPanel onSelectDiff={handleSelectDiff} />;
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

// --- LEFT HEADER ACTIONS (renders after last tab — "+" button) ---
function LeftHeaderActions(props: IDockviewHeaderActionsProps) {
  const { group, containerApi } = props;
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);

  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);
  const addFilesPanel = useDockviewStore((s) => s.addFilesPanel);
  const addChangesPanel = useDockviewStore((s) => s.addChangesPanel);

  // Single-instance panels: hide from menu when already open
  const hasChanges = Boolean(containerApi.getPanel('changes') ?? containerApi.getPanel('diff-files'));
  const hasFiles = Boolean(containerApi.getPanel('files') ?? containerApi.getPanel('all-files'));

  const isSidebarGroup = group.id === sidebarGroupId;

  const handleAddTerminal = useCallback(async () => {
    if (!activeSessionId) return;
    try {
      const result = await createUserShell(activeSessionId);
      addTerminalPanel(result.terminalId, group.id);
    } catch (error) {
      console.error('Failed to create terminal:', error);
    }
  }, [activeSessionId, addTerminalPanel, group.id]);

  // No "+" for the sidebar
  if (isSidebarGroup) return null;

  return (
    <div className="flex items-center gap-1 pl-1">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 w-6 p-0 cursor-pointer"
          >
            <IconPlus className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-44">
          <DropdownMenuItem onClick={handleAddTerminal} className="cursor-pointer text-xs">
            <IconTerminal2 className="h-3.5 w-3.5 mr-1.5" />
            Terminal
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => addBrowserPanel(undefined, group.id)} className="cursor-pointer text-xs">
            <IconDeviceDesktop className="h-3.5 w-3.5 mr-1.5" />
            Browser
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => addPlanPanel(group.id)} className="cursor-pointer text-xs">
            <IconFileText className="h-3.5 w-3.5 mr-1.5" />
            Plan
          </DropdownMenuItem>
          {!hasChanges && (
            <DropdownMenuItem onClick={() => addChangesPanel(group.id)} className="cursor-pointer text-xs">
              <IconGitBranch className="h-3.5 w-3.5 mr-1.5" />
              Changes
            </DropdownMenuItem>
          )}
          {!hasFiles && (
            <DropdownMenuItem onClick={() => addFilesPanel(group.id)} className="cursor-pointer text-xs">
              <IconFolder className="h-3.5 w-3.5 mr-1.5" />
              Files
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

// --- RIGHT HEADER ACTIONS (sidebar: "+ Task", center: browser preview, terminal: run scripts) ---
function RightHeaderActions(props: IDockviewHeaderActionsProps) {
  const { group } = props;
  const centerGroupId = useDockviewStore((s) => s.centerGroupId);
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);
  const rightBottomGroupId = useDockviewStore((s) => s.rightBottomGroupId);

  const isSidebarGroup = group.id === sidebarGroupId;
  const isCenterGroup = group.id === centerGroupId;
  const isTerminalGroup = group.id === rightBottomGroupId;

  if (isSidebarGroup) {
    return <SidebarRightActions />;
  }

  if (isCenterGroup) {
    return <CenterRightActions />;
  }

  if (isTerminalGroup) {
    return <TerminalGroupRightActions />;
  }

  return null;
}

function SidebarRightActions() {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const kanban = useAppStore((state) => state.kanban);
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const appStore = useAppStoreApi();
  const steps = (kanban?.steps ?? []).map((s: { id: string; title: string; color?: string; events?: { on_enter?: Array<{ type: string; config?: Record<string, unknown> }>; on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }> } }) => ({
    id: s.id,
    title: s.title,
    color: s.color,
    events: s.events,
  }));

  const handleTaskCreated = useCallback(
    (task: Task, _mode: 'create' | 'edit', meta?: { taskSessionId?: string | null }) => {
      const oldSessionId = appStore.getState().tasks.activeSessionId;
      setActiveTask(task.id);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
        performLayoutSwitch(oldSessionId, meta.taskSessionId);
        window.history.replaceState({}, '', linkToSession(meta.taskSessionId));
      }
    },
    [setActiveTask, setActiveSession, appStore]
  );

  return (
    <div className="flex items-center pr-2">
      <NewTaskButton
        workspaceId={workspaceId}
        workflowId={workflowId}
        steps={steps}
        onSuccess={handleTaskCreated}
      />
    </div>
  );
}

function CenterRightActions() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const repository = useAppStore((state) => {
    if (!activeSessionId) return null;
    const session = state.taskSessions.items[activeSessionId];
    if (!session) return null;
    const repoId = session.repository_id;
    if (!repoId) return null;
    const allRepos = Object.values(state.repositories.itemsByWorkspaceId).flat();
    return allRepos.find((r) => r.id === repoId) ?? null;
  });
  const hasDevScript = Boolean(repository?.dev_script?.trim());

  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);

  const handleStartBrowser = useCallback(async () => {
    addBrowserPanel();

    if (hasDevScript && activeSessionId) {
      try {
        const resp = await startProcess(activeSessionId, { kind: 'dev' });
        if (resp?.process) {
          upsertProcessStatus(mapProcessToStatus(resp.process));
          setActiveProcess(resp.process.session_id, resp.process.id);
        }
      } catch {
        // Process may already be running
      }
    }
  }, [addBrowserPanel, hasDevScript, activeSessionId, upsertProcessStatus, setActiveProcess]);

  if (!hasDevScript) return null;

  return (
    <div className="flex items-center gap-1 pr-1">
      <Button
        size="sm"
        variant="ghost"
        className="h-6 w-6 p-0 cursor-pointer"
        onClick={handleStartBrowser}
        title="Open browser preview"
      >
        <IconDeviceDesktop className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function TerminalGroupRightActions() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const repositoryId = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.repository_id ?? null;
  });
  const hasDevScript = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return false;
    const repoId = state.taskSessions.items[sessionId]?.repository_id;
    if (!repoId) return false;
    const allRepos = Object.values(state.repositories.itemsByWorkspaceId).flat();
    const repo = allRepos.find((r) => r.id === repoId);
    return Boolean(repo?.dev_script?.trim());
  });

  const { scripts } = useRepositoryScripts(repositoryId);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);
  const rightBottomGroupId = useDockviewStore((s) => s.rightBottomGroupId);

  const handleRunScript = useCallback(async (scriptId: string) => {
    if (!activeSessionId) return;
    try {
      const result = await createUserShell(activeSessionId, scriptId);
      addTerminalPanel(result.terminalId, rightBottomGroupId);
    } catch (error) {
      console.error('Failed to run script:', error);
    }
  }, [activeSessionId, addTerminalPanel, rightBottomGroupId]);

  const handleStartPreview = useCallback(async () => {
    if (!activeSessionId) return;
    addBrowserPanel();
    try {
      // Start dev process
      const resp = await startProcess(activeSessionId, { kind: 'dev' });
      if (resp?.process) {
        upsertProcessStatus(mapProcessToStatus(resp.process));
        setActiveProcess(resp.process.session_id, resp.process.id);
      }
    } catch {
      // Process may already be running
    }
    try {
      // Also create a terminal for the dev server output
      const shell = await createUserShell(activeSessionId);
      addTerminalPanel(shell.terminalId, rightBottomGroupId);
    } catch {
      // Terminal creation is best-effort
    }
  }, [activeSessionId, addBrowserPanel, upsertProcessStatus, setActiveProcess, addTerminalPanel, rightBottomGroupId]);

  if (scripts.length === 0 && !hasDevScript) return null;

  return (
    <div className="flex items-center gap-1 pr-1">
      {scripts.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              className="h-6 w-6 p-0 cursor-pointer"
              title="Run script"
            >
              <IconPlayerPlay className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
            {scripts.map((script) => (
              <DropdownMenuItem
                key={script.id}
                onClick={() => handleRunScript(script.id)}
                className="cursor-pointer text-xs"
              >
                <IconTerminal2 className="h-3.5 w-3.5 mr-1.5 shrink-0" />
                <span className="truncate">{script.name}</span>
                <span className="ml-auto text-muted-foreground font-mono text-[10px] truncate max-w-[120px]">
                  {script.command}
                </span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {hasDevScript && (
        <Button
          size="sm"
          variant="ghost"
          className="h-6 w-6 p-0 cursor-pointer"
          onClick={handleStartPreview}
          title="Start dev server preview"
        >
          <IconDeviceDesktop className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );
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

  // Connect LSP Go-to-Definition navigation to dockview file tabs
  useLspFileOpener();

  // Keep sessionIdRef in sync for use inside event handlers
  useEffect(() => {
    sessionIdRef.current = effectiveSessionId;
  }, [effectiveSessionId]);

  const onReady = useCallback(
    (event: DockviewReadyEvent) => {
      const api = event.api;
      setApi(api);

      // Restore chain: per-session → global localStorage (only if no session) → default
      let restored = false;
      const currentSessionId = sessionIdRef.current;

      // 1. Try per-session layout
      if (currentSessionId) {
        try {
          const sessionLayout = getSessionLayout(currentSessionId);
          if (sessionLayout) {
            api.fromJSON(sessionLayout as SerializedDockview);
            useDockviewStore.setState(applyLayoutFixups(api));
            restored = true;
          }
        } catch {
          // Per-session restore failed, try global
        }
      }

      // 2. Fallback to global localStorage — only when there's no session context
      //    (true first load). If we have a session ID but no saved layout, it's a
      //    new session and should get the default layout, not an old global one.
      if (!restored && !currentSessionId) {
        try {
          const saved = localStorage.getItem(LAYOUT_STORAGE_KEY);
          if (saved) {
            const layout = JSON.parse(saved);
            api.fromJSON(layout);
            useDockviewStore.setState(applyLayoutFixups(api));
            restored = true;
          }
        } catch {
          // Global restore failed, build default
        }
      }

      // 3. Fallback to default layout
      if (!restored) {
        buildDefaultLayout(api);
      }

      // Mark which session this initial layout is for
      useDockviewStore.setState({ currentLayoutSessionId: currentSessionId });

      // Track active group
      api.onDidActiveGroupChange((group) => {
        useDockviewStore.setState({ activeGroupId: group?.id ?? null });
      });
      useDockviewStore.setState({ activeGroupId: api.activeGroup?.id ?? null });

      // Enforce column max widths after group add/remove (dockview redistributes all columns equally)
      const enforceColumnMaxWidths = () => {
        requestAnimationFrame(() => {
          try {
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            const rootSplitview = (api as any).component?.gridview?.root?.splitview;
            if (!rootSplitview?.resizeView || !rootSplitview?.getViewSize) return;
            const count = rootSplitview.length;
            if (count < 3) return;

            const sidebarMax = Math.min(Math.round(api.width * LAYOUT_SIDEBAR_RATIO), LAYOUT_SIDEBAR_MAX_PX);
            const rightMax = Math.min(Math.round(api.width * LAYOUT_RIGHT_RATIO), LAYOUT_RIGHT_MAX_PX);

            // Clamp sidebar and right columns
            if (rootSplitview.getViewSize(0) > sidebarMax) {
              rootSplitview.resizeView(0, sidebarMax);
            }
            if (rootSplitview.getViewSize(count - 1) > rightMax) {
              rootSplitview.resizeView(count - 1, rightMax);
            }

            // Equalize center columns (everything between sidebar and right)
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
      };
      api.onDidAddGroup(enforceColumnMaxWidths);
      api.onDidRemoveGroup(enforceColumnMaxWidths);

      // Safety net: re-add chat panel if it gets removed (keeps center group alive)
      api.onDidRemovePanel((panel) => {
        if (panel.id === 'chat') {
          // Skip during layout restore — fromJSON removes all panels before re-adding
          if (useDockviewStore.getState().isRestoringLayout) return;
          requestAnimationFrame(() => {
            if (api.getPanel('chat')) return;
            const sidebarPanel = api.getPanel('sidebar');
            api.addPanel({
              id: 'chat',
              component: 'chat',
              tabComponent: 'permanentTab',
              title: 'Chat',
              position: sidebarPanel
                ? { direction: 'right', referencePanel: 'sidebar' }
                : undefined,
            });
            const newChat = api.getPanel('chat');
            if (newChat) {
              useDockviewStore.setState({ centerGroupId: newChat.group.id });
            }
          });
        }
      });

      // Layout persistence: debounce save on every layout change
      api.onDidLayoutChange(() => {
        // Skip saves during layout restore to avoid persisting intermediate states
        if (useDockviewStore.getState().isRestoringLayout) return;

        if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
        saveTimerRef.current = setTimeout(() => {
          try {
            const json = api.toJSON();
            // Save to global localStorage (fallback for initial page load)
            localStorage.setItem(LAYOUT_STORAGE_KEY, JSON.stringify(json));
            // Also save to per-session storage
            const sid = sessionIdRef.current;
            if (sid) {
              setSessionLayout(sid, json);
            }
          } catch {
            // Ignore serialization errors
          }
        }, 300);
      });
    },
    [setApi, buildDefaultLayout]
  );

  // Catch-all: detect session changes (e.g. from router.push after session creation)
  // and trigger layout switch if not already handled by a manual performLayoutSwitch call.
  // The idempotency check in switchSessionLayout (via currentLayoutSessionId) prevents
  // double-switching when callsites already call performLayoutSwitch synchronously.
  const prevSessionRef = useRef<string | null | undefined>(undefined);
  useEffect(() => {
    // undefined = first render, skip (onReady handles initial restore)
    if (prevSessionRef.current === undefined) {
      prevSessionRef.current = effectiveSessionId;
      return;
    }
    // No change
    if (prevSessionRef.current === effectiveSessionId) return;

    const oldSessionId = prevSessionRef.current;
    prevSessionRef.current = effectiveSessionId;

    if (effectiveSessionId) {
      performLayoutSwitch(oldSessionId, effectiveSessionId);
    }
  }, [effectiveSessionId]);

  // Clean up timer on unmount
  useEffect(() => {
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
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
    </div>
  );
});
