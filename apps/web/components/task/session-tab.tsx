"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import { IconStar } from "@tabler/icons-react";
import { AgentLogo } from "@/components/agent-logo";
import { GridSpinner } from "@/components/grid-spinner";
import { ContextMenu, ContextMenuTrigger } from "@kandev/ui/context-menu";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { renameSession } from "@/lib/api/domains/session-api";
import {
  useSessionActions,
  isSessionDeletable as isDeletable,
} from "@/hooks/domains/session/use-session-actions";
import { shareableSessionStateClient } from "@/components/task/share/share-button";
import type { HandoffPreset } from "@/components/task/new-session-dialog";
import { usableConfigOptions } from "@/components/model-config-selector";
import type { AppState } from "@/lib/state/store";
import { SessionContextMenuItems, SessionTabDialogs } from "./session-tab-menu";
import type { TaskSessionState } from "@/lib/types/http";
import {
  markSessionTabUserActivationIntent,
  shouldMarkSessionTabUserActivationIntent,
} from "./session-tab-activation-intent";
import {
  isSessionActive,
  resolveWorkflowStepTitle,
  splitAgentProfileLabel,
} from "@/lib/state/slices/session/session-sort";
import { resolveSessionTabTitle, resolveSnapshotModel } from "./session-tab-title";
import { TabRenameInput } from "./tab-rename-input";
import { useTabMaximizeOnDoubleClick } from "./use-tab-maximize";

function sessionRank(state: AppState, sessionId: string): number | null {
  const activeTaskId = state.tasks.activeTaskId;
  const sessions = activeTaskId ? state.taskSessionsByTask.itemsByTaskId[activeTaskId] : null;
  const index = sessions?.findIndex((s: { id: string }) => s.id === sessionId) ?? -1;
  return index >= 0 ? index + 1 : null;
}

function agentTabLabel(state: AppState, agentProfileId: string | undefined): string | null {
  if (!agentProfileId) return null;
  const profile = state.agentProfiles.items.find((p: { id: string }) => p.id === agentProfileId);
  return splitAgentProfileLabel(profile);
}

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
    const stepLabel = resolveWorkflowStepTitle(state, session?.workflow_step_id);
    const agentLabel = agentTabLabel(state, session?.agent_profile_id);
    return resolveSessionTabTitle({
      customName: session?.name ?? null,
      stepLabel,
      agentLabel,
      rank: sessionRank(state, sessionId),
      activeModelId,
      currentModelId: sessionModels?.currentModelId || null,
      snapshotModel: resolveSnapshotModel(session?.agent_profile_snapshot),
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
    // The stored list is kept in workflow-step-flow order (see
    // reorderStoredSessions in the session slice), so the badge is simply the
    // session's 1-based position in that list — guaranteeing the badge number
    // always matches the visible left-to-right tab order. Shares sessionRank's
    // lookup so the tab title and the badge can never disagree.
    return sessionRank(state, sessionId);
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
          title: "Rename failed",
          description: error instanceof Error ? error.message : "Unknown error",
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
  const onDeleted = useCallback(() => {
    const panel = containerApi.getPanel(api.id);
    if (panel) containerApi.removePanel(panel);
  }, [api.id, containerApi]);
  const {
    setPrimary: handleSetPrimary,
    stop: handleStop,
    resume: handleResume,
    remove: handleDelete,
  } = useSessionActions({ sessionId, taskId, onDeleted });
  const handleCloseOthers = useCallback(() => {
    const toClose = api.group.panels.filter((p) => p.id !== api.id);
    for (const panel of toClose) containerApi.removePanel(panel);
  }, [api, containerApi]);
  return { handleSetPrimary, handleStop, handleResume, handleDelete, handleCloseOthers };
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

function SessionTabTriggerContent({
  props,
  sessionId,
  isPrimary,
  showMultiSessionBadges,
  agentName,
  sessionState,
  isActive,
  showDeleteOnClose,
  onCloseTab,
}: {
  props: IDockviewPanelHeaderProps;
  sessionId: string | undefined;
  isPrimary: boolean;
  showMultiSessionBadges: boolean;
  agentName: string | null;
  sessionState: TaskSessionState | null;
  isActive: boolean;
  showDeleteOnClose: boolean;
  onCloseTab: () => void;
}) {
  const tabContentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!showDeleteOnClose || !sessionId) return;
    const closeAction = tabContentRef.current?.querySelector(".dv-default-tab-action");
    if (!closeAction) return;
    closeAction.setAttribute("data-testid", `session-tab-close-${sessionId}`);
    return () => closeAction.removeAttribute("data-testid");
  }, [showDeleteOnClose, sessionId, isActive]); // isActive: re-run when tab activates so Dockview renders .dv-default-tab-action

  return (
    <div ref={tabContentRef} className="flex items-center">
      {isPrimary && showMultiSessionBadges && (
        <IconStar className="h-3 w-3 fill-foreground/50 stroke-0 shrink-0 ml-2" />
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
      <DockviewDefaultTab
        {...props}
        hideClose={!showDeleteOnClose}
        closeActionOverride={showDeleteOnClose ? onCloseTab : undefined}
      />
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
  agentName: string | null;
  sessionState: TaskSessionState | null;
  isActive: boolean;
  showDeleteOnClose: boolean;
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

/**
 * Custom dockview tab for session panels.
 * Shows agent logo and star for primary (rank is already embedded in the tab
 * title via `#<rank>`, so no separate numeric badge is rendered); right-click
 * for lifecycle actions.
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
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const canShare = !!taskId && !!sessionId && shareableSessionStateClient(sessionState);

  useEffect(() => {
    // Always call setTitle (not gated on api.title !== tabTitle) when tabTitle
    // changes: dockview's setTitle already no-ops internally when the value is
    // unchanged (see DockviewPanelModel.setTitle), so this is cheap, and
    // reading api.title here to skip the call is unreliable across dockview
    // panel moves/reconciliation (dockview-react's own useTitle hook has a
    // similar comment: "the title may already be out of sync, cf. issue
    // #1003"). Always syncing guarantees the tab strip never gets stuck on a
    // stale (e.g. rank-less) title after a session is added/reordered.
    if (tabTitle) api.setTitle(tabTitle);
  }, [tabTitle, api]);

  const showMultiSessionBadges = sessionCount > 1;
  // Multi-session tab close means delete, not hide-only. Running/starting sessions are
  // not deletable, so we omit the X rather than reviving hide-only close behavior.
  const showDeleteOnClose = showMultiSessionBadges && !!sessionState && isDeletable(sessionState);
  const { setConfirmDelete, setIsRenaming, setShareOpen } = dialogs;
  const handleCloseTab = useCallback(() => {
    setConfirmDelete(true);
  }, [setConfirmDelete]);
  const { handlePointerDownCapture, handleKeyDownCapture } = useSessionTabUserActivationIntent(
    sessionId,
    activeSessionId,
    isActive,
  );

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger
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
            onCancelRename={() => setIsRenaming(false)}
            sessionId={sessionId}
            isPrimary={isPrimary}
            showMultiSessionBadges={showMultiSessionBadges}
            agentName={agentName}
            sessionState={sessionState}
            isActive={isActive}
            showDeleteOnClose={showDeleteOnClose}
            onCloseTab={handleCloseTab}
          />
        </ContextMenuTrigger>
        <SessionContextMenuItems
          sessionState={sessionState}
          isPrimary={isPrimary}
          canShare={canShare}
          taskId={taskId}
          sessionId={sessionId}
          actions={actions}
          onDelete={() => setConfirmDelete(true)}
          onShare={() => setShareOpen(true)}
          onHandoffProfile={dialogs.handleHandoffProfile}
          onStartRename={() => setIsRenaming(true)}
        />
      </ContextMenu>
      <SessionTabDialogs
        confirmDelete={dialogs.confirmDelete}
        setConfirmDelete={setConfirmDelete}
        isPrimary={isPrimary}
        sessionCount={sessionCount}
        onConfirmDelete={actions.handleDelete}
        taskId={taskId}
        sessionId={sessionId}
        shareOpen={dialogs.shareOpen}
        setShareOpen={setShareOpen}
        handoffOpen={dialogs.handoffOpen}
        setHandoffOpen={dialogs.setHandoffOpen}
        handoffPreset={dialogs.handoffPreset}
        setHandoffPreset={dialogs.setHandoffPreset}
        groupId={api.group?.id}
      />
    </>
  );
}
