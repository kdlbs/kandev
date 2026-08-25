"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
} from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { IconStar } from "@tabler/icons-react";
import { AgentLogo } from "@/components/agent-logo";
import { GridSpinner } from "@/components/grid-spinner";
import { ContextMenu, ContextMenuTrigger } from "@kandev/ui/context-menu";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { renameSession } from "@/lib/api/domains/session-api";
import { useSessionActions } from "@/hooks/domains/session/use-session-actions";
import { shareableSessionStateClient } from "@/components/task/share/share-button";
import type { HandoffPreset } from "@/components/task/new-session-dialog";
import { usableConfigOptions } from "@/components/model-config-selector";
import { SessionContextMenuItems, SessionTabDialogs } from "./session-tab-menu";
import type { TaskSessionState } from "@/lib/types/http";
import {
  markSessionTabUserActivationIntent,
  shouldMarkSessionTabUserActivationIntent,
} from "./session-tab-activation-intent";
import { isSessionActive } from "./session-sort";
import { resolveSessionTabTitle } from "./session-tab-title";
import { TabRenameInput } from "./tab-rename-input";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";
import { SessionTabCloseAction } from "./session-tab-close-action";
import { useSessionTabDelete } from "./use-session-tab-delete";
import { useTranslation } from "react-i18next";

function useSessionTabState(sessionId: string | undefined) {
  const isPrimary = useAppStore((state) => {
    const activeTaskId = state.tasks.activeTaskId;
    if (!activeTaskId || !sessionId) return false;
    const task = state.kanban.tasks.find((t: { id: string }) => t.id === activeTaskId);
    if (task?.primarySessionId) return task.primarySessionId === sessionId;
    return state.taskSessions.items[sessionId]?.is_primary === true;
  });
  const sessionState = useAppStore((state) => {
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.state ?? null;
  }) as TaskSessionState | null;
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const sessionName = useAppStore((state) => {
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.name ?? null;
  });
  const tabTitle = useAppStore((state) => {
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    const sessionModels = state.sessionModels.bySessionId[sessionId];
    const activeModelId = state.activeModel.bySessionId[sessionId] || null;
    const agentLabel = (() => {
      if (!session?.agent_profile_id) return null;
      const profile = state.agentProfiles.items.find(
        (p: { id: string }) => p.id === session.agent_profile_id,
      );
      if (!profile) return null;
      const parts = profile.label.split(" \u2022 ");
      return parts[1] || parts[0] || profile.label;
    })();
    const snapshotModel =
      typeof session?.agent_profile_snapshot?.model === "string"
        ? session.agent_profile_snapshot.model
        : null;
    return resolveSessionTabTitle({
      customName: session?.name ?? null,
      agentLabel,
      activeModelId,
      currentModelId: sessionModels?.currentModelId || null,
      snapshotModel,
      modelOptions:
        sessionModels?.models.map((model) => ({
          id: model.modelId,
          name: model.name,
          description: model.description,
          usageMultiplier: model.usageMultiplier,
        })) ?? [],
      configOptions: usableConfigOptions(sessionModels?.configOptions),
    });
  });
  const agentName = useAppStore((state) => {
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    if (!session?.agent_profile_id) return null;
    return (
      state.agentProfiles.items.find((p: { id: string }) => p.id === session.agent_profile_id)
        ?.agent_name ?? null
    );
  });
  const sessionNumber = useAppStore((state) => {
    if (!sessionId) return null;
    const activeTaskId = state.tasks.activeTaskId;
    const sessions = activeTaskId ? state.taskSessionsByTask.itemsByTaskId[activeTaskId] : null;
    if (!sessions) return null;
    // Sort chronologically (oldest first) so indexes are stable regardless of
    // which session is primary or the backend's default DESC ordering.
    const sorted = [...sessions].sort(
      (a: { started_at: string }, b: { started_at: string }) =>
        new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
    );
    const idx = sorted.findIndex((s: { id: string }) => s.id === sessionId);
    return idx >= 0 ? idx + 1 : null;
  });
  const sessionCount = useAppStore((state) => {
    const activeTaskId = state.tasks.activeTaskId;
    if (!activeTaskId) return 0;
    return state.taskSessionsByTask.itemsByTaskId[activeTaskId]?.length ?? 0;
  });
  return {
    isPrimary,
    sessionState,
    taskId,
    tabTitle,
    sessionName,
    agentName,
    sessionNumber,
    sessionCount,
  };
}

/** Mirrors the backend's maxSessionNameLength so the optimistic store update
 * matches what the rename broadcast will echo back. */
const MAX_SESSION_NAME_LENGTH = 120;

/** Commit a session tab rename: persist via WS and optimistically update the store. */
function useSessionRenameCommitter(
  sessionId: string | undefined,
  taskId: string | null,
  currentName: string | null,
  onDone: () => void,
) {
  const { t } = useTranslation();
  const appStoreApi = useAppStoreApi();
  const { toast } = useToast();
  return useCallback(
    async (next: string) => {
      onDone();
      if (!sessionId || !taskId) return;
      const normalized = next.trim().slice(0, MAX_SESSION_NAME_LENGTH);
      if ((currentName ?? "") === normalized) return;
      try {
        await renameSession(sessionId, normalized);
        const existing = appStoreApi.getState().taskSessions.items[sessionId];
        if (existing) {
          appStoreApi
            .getState()
            .upsertTaskSessionFromEvent(taskId, { ...existing, name: normalized });
        }
      } catch (error) {
        console.error("rename session:", error);
        toast({
          title: t("task:renameFailed"),
          description: error instanceof Error ? error.message : t("common:unknownError"),
          variant: "error",
        });
      }
    },
    [sessionId, taskId, currentName, appStoreApi, onDone, toast],
  );
}

function useSessionTabActions(
  sessionId: string | undefined,
  taskId: string | null,
  api: IDockviewPanelHeaderProps["api"],
  containerApi: IDockviewPanelHeaderProps["containerApi"],
) {
  const handleCloseTab = useCallback(() => {
    const panel = containerApi.getPanel(api.id);
    if (!panel) return;

    // When closing the active session panel and the successor would be a
    // non-session panel (PR, Plan, File tabs), explicitly activate another
    // session panel in the same group before removal. This prevents Dockview
    // from activating a non-session panel, which would leave
    // activeSessionId pointing to the now-closed panel and cause
    // runAutoSessionTabEffect to recreate it.
    const isActive = panel.api.isActive;
    if (isActive) {
      const siblingSessionPanel = containerApi.panels.find(
        (p) => p.id !== api.id && p.id.startsWith("session:"),
      );
      if (siblingSessionPanel) {
        siblingSessionPanel.api.setActive();
      }
    }

    containerApi.removePanel(panel);
  }, [api.id, containerApi]);

  const {
    setPrimary: handleSetPrimary,
    stop: handleStop,
    resume: handleResume,
    remove: handleDelete,
  } = useSessionActions({ sessionId, taskId, onDeleted: handleCloseTab });
  const handleCloseOthers = useCallback(() => {
    const toClose = containerApi.panels.filter(
      (panel) => panel.id !== api.id && panel.id.startsWith("session:"),
    );
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api.id, containerApi]);
  return {
    handleSetPrimary,
    handleStop,
    handleResume,
    handleDelete,
    handleCloseTab,
    handleCloseOthers,
  };
}

function useSessionTabUserActivationIntent(
  sessionId: string | undefined,
  activeSessionId: string | null,
  isActive: boolean,
) {
  const markUserActivationIntent = useCallback(
    (target: EventTarget | null) => {
      if (
        !shouldMarkSessionTabUserActivationIntent({ sessionId, activeSessionId, isActive, target })
      )
        return;
      markSessionTabUserActivationIntent(sessionId);
    },
    [activeSessionId, isActive, sessionId],
  );
  const handlePointerDownCapture = useCallback(
    (event: ReactPointerEvent) => {
      if (event.button === 0) markUserActivationIntent(event.target);
    },
    [markUserActivationIntent],
  );
  const handleKeyDownCapture = useCallback(
    (event: ReactKeyboardEvent) => {
      if (event.key === "Enter" || event.key === " ") markUserActivationIntent(event.target);
    },
    [markUserActivationIntent],
  );
  return { handlePointerDownCapture, handleKeyDownCapture };
}

function useDockviewTabActiveState(api: IDockviewPanelHeaderProps["api"]) {
  const [isActive, setIsActive] = useState(api.isActive);
  useEffect(() => {
    const disposable = api.onDidActiveChange((e) => setIsActive(e.isActive));
    return () => disposable.dispose();
  }, [api]);
  return isActive;
}

function countVisibleSessionPanels(
  containerApi: IDockviewPanelHeaderProps["containerApi"],
  removedPanelId?: string,
): number {
  return containerApi.panels.filter(
    (panel) => panel.id !== removedPanelId && panel.id.startsWith("session:"),
  ).length;
}

function useVisibleSessionPanelCount(
  containerApi: IDockviewPanelHeaderProps["containerApi"],
): number {
  const [count, setCount] = useState(() => countVisibleSessionPanels(containerApi));
  useEffect(() => {
    const addDisposable = containerApi.onDidAddPanel(() => {
      setCount(countVisibleSessionPanels(containerApi));
    });
    const removeDisposable = containerApi.onDidRemovePanel((panel) => {
      setCount(countVisibleSessionPanels(containerApi, panel.id));
    });
    return () => {
      addDisposable.dispose();
      removeDisposable.dispose();
    };
  }, [containerApi]);
  return count;
}

function SessionTabTriggerContent({
  props,
  sessionId,
  isPrimary,
  showMultiSessionBadges,
  sessionNumber,
  agentName,
  sessionState,
  isActive,
  showCloseAction,
  onCloseTab,
}: {
  props: IDockviewPanelHeaderProps;
  sessionId: string | undefined;
  isPrimary: boolean;
  showMultiSessionBadges: boolean;
  sessionNumber: number | null;
  agentName: string | null;
  sessionState: TaskSessionState | null;
  isActive: boolean;
  showCloseAction: boolean;
  onCloseTab: () => void;
}) {
  return (
    <div className="flex items-center">
      {isPrimary && showMultiSessionBadges && (
        <IconStar className="h-3 w-3 fill-foreground/50 stroke-0 shrink-0 ml-2" />
      )}
      {sessionNumber != null && showMultiSessionBadges && (
        <span className="ml-1.5 text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5">
          {sessionNumber}
        </span>
      )}
      {agentName &&
        (isSessionActive(sessionState) ? (
          <GridSpinner
            className={`ml-1.5 shrink-0 text-[14px] text-muted-foreground${isActive ? "" : " opacity-50"}`}
          />
        ) : (
          <AgentLogo
            agentName={agentName}
            size={14}
            className={`ml-1.5 shrink-0${isActive ? "" : " opacity-50"}`}
          />
        ))}
      <DockviewDefaultTab {...props} hideClose />
      {showCloseAction && <SessionTabCloseAction sessionId={sessionId} onClose={onCloseTab} />}
    </div>
  );
}

/** Bundles the open/close state for the tab's dialogs and inline rename. */
function useSessionTabDialogState(sessionId: string | undefined) {
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [isRenaming, setIsRenaming] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const [handoffOpen, setHandoffOpen] = useState(false);
  const [handoffPreset, setHandoffPreset] = useState<HandoffPreset | null>(null);
  const handleHandoffProfile = useCallback(
    (profileId: string) => {
      if (!sessionId) return;
      setHandoffPreset({ sourceSessionId: sessionId, targetProfileId: profileId });
      setHandoffOpen(true);
    },
    [sessionId],
  );
  return {
    confirmDelete,
    setConfirmDelete,
    isRenaming,
    setIsRenaming,
    shareOpen,
    setShareOpen,
    handoffOpen,
    setHandoffOpen,
    handoffPreset,
    setHandoffPreset,
    handleHandoffProfile,
  };
}

function useSessionMenuDelete(
  triggerRef: RefObject<HTMLElement | null>,
  openMenuDelete: () => void,
) {
  const menuDeleteAnchorRef = useRef<HTMLElement>(null);
  const menuDeleteFocusBoundaryRef = useRef<HTMLElement>(null);
  const handleMenuDelete = useCallback(
    (_event: Event) => {
      // Use the tab trigger element as the popover anchor instead of the
      // context menu item. The context menu item becomes disconnected from the
      // DOM when the context menu closes on the popover interaction, causing
      // ActionConfirmPopover.handleConfirm isConnected check to reject the
      // confirm click.
      menuDeleteAnchorRef.current = triggerRef.current;
      menuDeleteFocusBoundaryRef.current =
        triggerRef.current?.closest('[data-slot="context-menu-content"]') ?? null;
      openMenuDelete();
    },
    [openMenuDelete, triggerRef],
  );
  return { menuDeleteAnchorRef, menuDeleteFocusBoundaryRef, handleMenuDelete };
}

/** Tab body: inline rename input while renaming, normal trigger content otherwise. */
function SessionTabBody({
  props,
  isRenaming,
  renameInitial,
  renameSeqBadge,
  onCommitRename,
  onCancelRename,
  ...contentProps
}: {
  props: IDockviewPanelHeaderProps;
  isRenaming: boolean;
  renameInitial: string;
  renameSeqBadge: number | null;
  onCommitRename: (next: string) => void;
  onCancelRename: () => void;
  sessionId: string | undefined;
  isPrimary: boolean;
  showMultiSessionBadges: boolean;
  sessionNumber: number | null;
  agentName: string | null;
  sessionState: TaskSessionState | null;
  isActive: boolean;
  showCloseAction: boolean;
  onCloseTab: () => void;
}) {
  if (isRenaming) {
    return (
      <TabRenameInput
        initial={renameInitial}
        seqBadge={renameSeqBadge}
        onCommit={onCommitRename}
        onCancel={onCancelRename}
        testId="session-tab-rename-input"
        maxLength={MAX_SESSION_NAME_LENGTH}
      />
    );
  }
  return <SessionTabTriggerContent props={props} {...contentProps} />;
}

/** Keeps the Dockview panel title synchronized when Dockview restores a stale title. */
function useSessionTabTitleSync(api: IDockviewPanelHeaderProps["api"], tabTitle: string | null) {
  useEffect(() => {
    const syncTitle = () => {
      if (tabTitle && api.title !== tabTitle) api.setTitle(tabTitle);
    };
    syncTitle();
    const disposable = api.onDidTitleChange(syncTitle);
    return () => disposable.dispose();
  }, [tabTitle, api]);
}

type SessionTabDialogHostProps = {
  dialogs: ReturnType<typeof useSessionTabDialogState>;
  deleteState: ReturnType<typeof useSessionTabDelete>;
  menuDeleteAnchorRef: RefObject<HTMLElement | null>;
  menuDeleteFocusBoundaryRef: RefObject<HTMLElement | null>;
  isPrimary: boolean;
  sessionCount: number;
  targetName: string | null;
  taskId: string | null;
  sessionId: string | undefined;
  groupId: string | undefined;
};

function SessionTabDialogHost({
  dialogs,
  deleteState,
  menuDeleteAnchorRef,
  menuDeleteFocusBoundaryRef,
  isPrimary,
  sessionCount,
  targetName,
  taskId,
  sessionId,
  groupId,
}: SessionTabDialogHostProps) {
  return (
    <SessionTabDialogs
      confirmDelete={dialogs.confirmDelete && deleteState.deleteOrigin === "tab"}
      menuDeleteOpen={dialogs.confirmDelete && deleteState.deleteOrigin === "menu"}
      menuDeleteAnchorRef={menuDeleteAnchorRef}
      menuDeleteFocusBoundaryRef={menuDeleteFocusBoundaryRef}
      setConfirmDelete={deleteState.handleDeleteDialogOpenChange}
      isPrimary={isPrimary}
      sessionCount={sessionCount}
      onConfirmDelete={deleteState.handleConfirmDelete}
      targetName={targetName}
      taskId={taskId}
      sessionId={sessionId}
      shareOpen={dialogs.shareOpen}
      setShareOpen={dialogs.setShareOpen}
      handoffOpen={dialogs.handoffOpen}
      setHandoffOpen={dialogs.setHandoffOpen}
      handoffPreset={dialogs.handoffPreset}
      setHandoffPreset={dialogs.setHandoffPreset}
      groupId={groupId}
    />
  );
}

/**
 * Custom dockview tab for session panels.
 * Shows agent logo, index badge, and star for primary; right-click for lifecycle actions.
 */
export function SessionTab(props: IDockviewPanelHeaderProps) {
  const { api, containerApi } = props;
  const sessionId = api.id.startsWith("session:") ? api.id.slice("session:".length) : undefined;
  const {
    isPrimary,
    sessionState,
    taskId,
    tabTitle,
    sessionName,
    agentName,
    sessionNumber,
    sessionCount,
  } = useSessionTabState(sessionId);
  const actions = useSessionTabActions(sessionId, taskId, api, containerApi);
  const onDoubleClick = useTabMaximizeOnDoubleClick(api);
  const dialogs = useSessionTabDialogState(sessionId);
  const handleCommitRename = useSessionRenameCommitter(sessionId, taskId, sessionName, () =>
    dialogs.setIsRenaming(false),
  );
  const isActive = useDockviewTabActiveState(api);
  const visibleSessionPanelCount = useVisibleSessionPanelCount(containerApi);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const canShare = !!taskId && !!sessionId && shareableSessionStateClient(sessionState);

  useSessionTabTitleSync(api, tabTitle);

  const showMultiSessionBadges = sessionCount > 1;
  const showCloseAction = visibleSessionPanelCount > 1;
  const deleteState = useSessionTabDelete(dialogs.setConfirmDelete, actions.handleDelete);
  const triggerRef = useRef<HTMLElement>(null);
  const { menuDeleteAnchorRef, menuDeleteFocusBoundaryRef, handleMenuDelete } =
    useSessionMenuDelete(triggerRef, deleteState.handleMenuDelete);
  const { handlePointerDownCapture, handleKeyDownCapture } = useSessionTabUserActivationIntent(
    sessionId,
    activeSessionId,
    isActive,
  );

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger
          ref={triggerRef as React.Ref<HTMLDivElement>}
          className="flex h-full items-center cursor-pointer select-none"
          data-testid={sessionId ? `session-tab-${sessionId}` : undefined}
          onPointerDownCapture={handlePointerDownCapture}
          onKeyDownCapture={handleKeyDownCapture}
          onDoubleClick={onDoubleClick}
        >
          <SessionTabBody
            props={props}
            isRenaming={dialogs.isRenaming}
            renameInitial={sessionName || tabTitle || ""}
            renameSeqBadge={showMultiSessionBadges ? sessionNumber : null}
            onCommitRename={handleCommitRename}
            onCancelRename={() => dialogs.setIsRenaming(false)}
            sessionId={sessionId}
            isPrimary={isPrimary}
            showMultiSessionBadges={showMultiSessionBadges}
            sessionNumber={sessionNumber}
            agentName={agentName}
            sessionState={sessionState}
            isActive={isActive}
            showCloseAction={showCloseAction}
            onCloseTab={actions.handleCloseTab}
          />
        </ContextMenuTrigger>
        <SessionContextMenuItems
          sessionState={sessionState}
          isPrimary={isPrimary}
          canShare={canShare}
          taskId={taskId}
          sessionId={sessionId}
          actions={actions}
          onDelete={handleMenuDelete}
          onShare={() => dialogs.setShareOpen(true)}
          onHandoffProfile={dialogs.handleHandoffProfile}
          onStartRename={() => dialogs.setIsRenaming(true)}
        />
      </ContextMenu>
      <SessionTabDialogHost
        dialogs={dialogs}
        deleteState={deleteState}
        menuDeleteAnchorRef={menuDeleteAnchorRef}
        menuDeleteFocusBoundaryRef={menuDeleteFocusBoundaryRef}
        isPrimary={isPrimary}
        sessionCount={sessionCount}
        targetName={tabTitle}
        taskId={taskId}
        sessionId={sessionId}
        groupId={api.group?.id}
      />
    </>
  );
}
