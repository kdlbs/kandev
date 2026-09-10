import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskManagementFlow } from "@/hooks/use-task-management-flow";
import { useTaskMenuActions } from "@/hooks/use-task-menu-actions";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useUpdateTaskPriority } from "@/hooks/use-update-task-priority";
import type { TaskPriority } from "@/lib/types/http";
import { TaskManagementMenu, type TaskManagementMenuProps } from "./task-management-menu";
import { TaskManagementDrawer, TaskManagementSheet } from "./task-management-drawer";
import { TaskArchiveConfirmation } from "./task-archive-confirmation";
import { TaskDeleteConfirmDialog } from "./task-delete-confirm-dialog";
import { useSidebarLinkActions } from "./task-session-sidebar-link-actions";
import { useSidebarTaskLinking } from "./task-session-sidebar-task-linking";
import { SidebarLinkDialogs } from "./task-session-sidebar-dialogs";
import { selectTaskLinkActions } from "./task-switcher-link-menu";

type Flow = ReturnType<typeof useTaskManagementFlow>;
export type TaskMenuPoint = { x: number; y: number };
type SurfaceProps = {
  flow: Flow;
  point: TaskMenuPoint;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef: RefObject<HTMLElement | null>;
  onReturnFocus: () => void;
};

function DesktopMenu({
  point,
  onDismiss,
  onCloseAutoFocus,
  ...props
}: TaskManagementMenuProps & {
  point: TaskMenuPoint;
  onDismiss: () => void;
  onCloseAutoFocus: (event: Event) => void;
}) {
  const trigger = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    trigger.current?.dispatchEvent(
      new MouseEvent("contextmenu", { bubbles: true, clientX: point.x, clientY: point.y }),
    );
  }, [point]);
  return (
    <ContextMenu
      onOpenChange={(open) => {
        if (!open) onDismiss();
      }}
    >
      <ContextMenuTrigger asChild>
        <span ref={trigger} aria-hidden="true" className="pointer-events-none fixed size-0" />
      </ContextMenuTrigger>
      <ContextMenuContent
        data-testid="task-management-menu"
        className="w-56 max-w-[calc(100vw-16px)]"
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <TaskManagementMenu {...props} />
      </ContextMenuContent>
    </ContextMenu>
  );
}

function useManagementMutations(flow: Flow) {
  const removal = useTaskMenuActions({ stayOnListing: true });
  const updatePriority = useUpdateTaskPriority();
  const move = useTaskWorkflowMove();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const run = async (operation: () => Promise<unknown>) => {
    if (pending.current || removal.pendingTaskId) return;
    pending.current = true;
    setBusy(true);
    flow.close();
    try {
      await operation();
    } catch {
      /* Shared operation owns error feedback. */
    } finally {
      pending.current = false;
      setBusy(false);
    }
  };
  return {
    removal,
    disabled: busy || removal.pendingTaskId !== null,
    onPriority: (priority: TaskPriority) => {
      const task = flow.getTarget();
      if (task) void run(() => updatePriority(task.id, priority));
    },
    onMove: (workflowId: string, stepId: string) => {
      const task = flow.getTarget();
      const destination = flow.workflows.find((workflow) => workflow.id === workflowId);
      if (
        !task ||
        !destination ||
        destination.hidden ||
        !flow.stepsByWorkflowId[workflowId]?.some((step) => step.id === stepId)
      )
        return;
      if (task.workflowId === workflowId && task.workflowStepId === stepId) return;
      void run(() =>
        move([task.id], workflowId, stepId, workflowId === task.workflowId ? "step" : "workflow"),
      );
    },
  };
}

function useManagementLinks(flow: Flow) {
  const store = useAppStoreApi();
  const actions = useSidebarLinkActions(store);
  const handlers = useSidebarTaskLinking(flow.identity?.workspaceId ?? null, actions);
  const linking = Boolean(
    actions.linkingPullRequestTask ||
    actions.linkingIssueTask ||
    actions.linkingMergeRequestTask ||
    actions.linkingExternalIssueTask,
  );
  useEffect(() => {
    if (flow.stage === "link" && !linking) flow.close();
  }, [flow.stage, flow.close, linking]);
  const guardedHandlers = Object.fromEntries(
    Object.entries(handlers).map(([key, handler]) => [
      key,
      handler
        ? (taskId: string, title?: string) => {
            if (flow.getTarget()?.id === taskId) handler(taskId, title);
          }
        : undefined,
    ]),
  );
  const linkActions = flow.task
    ? selectTaskLinkActions(flow.task, () => flow.setStage("link"), guardedHandlers)
    : {};
  return { actions, linkActions };
}

function ManagementConfirmations({
  flow,
  mutations,
  anchorRef,
  focusReturnRef,
  onCloseAutoFocus,
  touch,
}: Pick<SurfaceProps, "flow" | "anchorRef" | "focusReturnRef"> & {
  mutations: ReturnType<typeof useManagementMutations>;
  onCloseAutoFocus: (event: Event) => void;
  touch: boolean;
}) {
  const { t } = useTranslation();
  const confirmationAnchor = useMemo(
    () => ({
      get current() {
        return anchorRef.current?.isConnected ? anchorRef.current : focusReturnRef.current;
      },
    }),
    [anchorRef, focusReturnRef],
  );
  const task = flow.task;
  if (!task) return null;
  return (
    <>
      <TaskArchiveConfirmation
        open={flow.stage === "archive"}
        onOpenChange={(open) => {
          if (!open) flow.close();
        }}
        taskId={task.id}
        taskTitle={task.title}
        executorType={task.remoteExecutorType}
        anchorRef={confirmationAnchor}
        focusReturnRef={focusReturnRef}
        isArchiving={mutations.disabled}
        inline={touch}
        renderInline={
          touch
            ? (content) => (
                <TaskManagementSheet
                  title={t("task:archiveTaskTitle")}
                  taskTitle={task.title}
                  onClose={flow.close}
                  onCloseAutoFocus={onCloseAutoFocus}
                >
                  {content}
                </TaskManagementSheet>
              )
            : undefined
        }
        onConfirm={async (opts) => {
          const target = flow.getTarget();
          if (!target) return;
          flow.close();
          await mutations.removal.runArchive(target.id, opts);
        }}
      />
      <TaskDeleteConfirmDialog
        open={flow.stage === "delete"}
        onOpenChange={(open) => {
          if (!open) flow.close();
        }}
        taskId={task.id}
        taskTitle={task.title}
        executorType={task.remoteExecutorType}
        isDeleting={mutations.disabled}
        onCloseAutoFocus={onCloseAutoFocus}
        confirmTestId="thread-delete-confirm"
        onConfirm={(opts) => {
          const target = flow.getTarget();
          if (target) void mutations.removal.runDelete(target.id, opts);
        }}
      />
    </>
  );
}

/** Owns task flows outside removable columns. All mutations and provider forms remain task-owned. */
export function TaskManagementSurface({
  flow,
  point,
  anchorRef,
  focusReturnRef,
  onReturnFocus,
}: SurfaceProps) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  const mutations = useManagementMutations(flow);
  const stageRef = useRef(flow.stage);
  const returnFocusRef = useRef(onReturnFocus);
  useLayoutEffect(() => {
    stageRef.current = flow.stage;
    returnFocusRef.current = onReturnFocus;
  });
  const onCloseAutoFocus = useCallback((event: Event) => {
    event.preventDefault();
    if (stageRef.current === "closed") returnFocusRef.current();
  }, []);
  useEffect(() => {
    if (flow.stage !== "closed" || !flow.identity) return;
    const frame = requestAnimationFrame(() => returnFocusRef.current());
    return () => cancelAnimationFrame(frame);
  }, [flow.stage, flow.identity]);
  return flow.task ? (
    <TaskManagementChoices
      key={`${flow.identity?.workspaceId}:${flow.identity?.taskId}`}
      flow={flow}
      point={point}
      anchorRef={anchorRef}
      focusReturnRef={focusReturnRef}
      mutations={mutations}
      touch={touch}
      onCloseAutoFocus={onCloseAutoFocus}
    />
  ) : null;
}

function TaskManagementChoices({
  flow,
  point,
  anchorRef,
  focusReturnRef,
  mutations,
  touch,
  onCloseAutoFocus,
}: Omit<SurfaceProps, "onReturnFocus"> & {
  mutations: ReturnType<typeof useManagementMutations>;
  touch: boolean;
  onCloseAutoFocus: (event: Event) => void;
}) {
  const links = useManagementLinks(flow);
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const closeMenu = useCallback(
    () => flow.setStage((stage) => (stage === "menu" ? "closed" : stage)),
    [flow.setStage],
  );
  const task = flow.task;
  const props = useMemo(
    () =>
      task
        ? {
            task,
            workflows: flow.workflows,
            stepsByWorkflowId: flow.stepsByWorkflowId,
            disabled: mutations.disabled,
            onPriority: mutations.onPriority,
            onMove: mutations.onMove,
            onArchive: () => flow.setStage("archive"),
            onDelete: () => flow.setStage("delete"),
            closeMenu,
            linkActions: links.linkActions,
          }
        : null,
    [
      task,
      flow.workflows,
      flow.stepsByWorkflowId,
      flow.setStage,
      mutations.disabled,
      mutations.onPriority,
      mutations.onMove,
      closeMenu,
      links.linkActions,
    ],
  );
  return (
    <>
      {flow.stage === "menu" &&
        props &&
        (touch ? (
          <TaskManagementDrawer key={task?.id} {...props} onCloseAutoFocus={onCloseAutoFocus} />
        ) : (
          <DesktopMenu
            {...props}
            point={point}
            onDismiss={closeMenu}
            onCloseAutoFocus={onCloseAutoFocus}
          />
        ))}
      <ManagementConfirmations
        flow={flow}
        mutations={mutations}
        anchorRef={anchorRef}
        focusReturnRef={focusReturnRef}
        onCloseAutoFocus={onCloseAutoFocus}
        touch={touch}
      />
      {flow.stage === "link" && (
        <SidebarLinkDialogs
          actions={links.actions}
          repositories={repositoriesByWorkspace[flow.identity?.workspaceId ?? ""] ?? []}
          workspaceId={flow.identity?.workspaceId ?? null}
        />
      )}
    </>
  );
}
