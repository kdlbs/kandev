import { createElement, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  IconArchive,
  IconArrowLeft,
  IconArrowRight,
  IconChevronRight,
  IconFlag,
  IconLink,
  IconLogicBuffer,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { cn } from "@/lib/utils";
import { TASK_PRIORITY_TOKENS, TASK_PRIORITY_LABEL_KEYS } from "@/lib/tasks/task-priority";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { taskMoveOptions, stepHasAutoStart, type TaskMoveStep } from "./task-move-context-menu";
import { taskLinkMenuOptions } from "./task-switcher-link-menu";
import { useTaskPluginLinkActions } from "./task-session-sidebar-link-actions";
import type { TaskManagementMenuProps } from "./task-management-menu";

export type TaskManagementDrawerProps = TaskManagementMenuProps & {
  onCloseAutoFocus: (event: Event) => void;
};
type Page = "root" | "priority" | "steps" | "workflows" | "links" | { workflowId: string };

export function TaskManagementSheet({
  title,
  taskTitle,
  onBack,
  onClose,
  onCloseAutoFocus,
  focusChoiceId,
  children,
}: {
  title: string;
  taskTitle: string;
  onBack?: () => void;
  onClose: () => void;
  onCloseAutoFocus: (event: Event) => void;
  focusChoiceId?: string;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const titleRef = useRef<HTMLHeadingElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const body = bodyRef.current;
    if (body) body.scrollTop = 0;
    const choice = focusChoiceId
      ? Array.from(body?.querySelectorAll<HTMLButtonElement>("button") ?? []).find(
          (button) => button.dataset.testid === focusChoiceId,
        )
      : undefined;
    (choice ?? titleRef.current)?.focus({ preventScroll: true });
    choice?.scrollIntoView?.({ block: "nearest" });
  }, [title, focusChoiceId]);
  return (
    <Drawer
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DrawerContent
        data-testid="task-management-drawer"
        className="min-w-0 !max-h-[min(80dvh,calc(100dvh-16px-env(safe-area-inset-bottom,0px)))] outline-none"
        onCloseAutoFocus={onCloseAutoFocus}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          titleRef.current?.focus({ preventScroll: true });
        }}
        onEscapeKeyDown={(event) => {
          if (onBack) {
            event.preventDefault();
            onBack();
          }
        }}
      >
        <div className="flex min-h-0 flex-col overflow-hidden rounded-xl bg-background">
          <DrawerHeader className="shrink-0 border-b text-left">
            <div className="flex min-w-0 items-center gap-2">
              {onBack && (
                <button
                  type="button"
                  className="flex size-11 shrink-0 items-center justify-center rounded-md hover:bg-accent"
                  aria-label={t("common:back")}
                  onClick={onBack}
                >
                  <IconArrowLeft className="size-5" />
                </button>
              )}
              <DrawerTitle
                ref={titleRef}
                tabIndex={-1}
                className="line-clamp-2 min-w-0 flex-1 text-left text-base outline-none [overflow-wrap:anywhere]"
              >
                {title}
              </DrawerTitle>
              <button
                type="button"
                className="flex size-11 shrink-0 items-center justify-center rounded-md hover:bg-accent"
                aria-label={t("common:close")}
                onClick={onClose}
              >
                <IconX className="size-5" />
              </button>
            </div>
            <DrawerDescription className="line-clamp-2 text-left [overflow-wrap:anywhere]">
              {taskTitle}
            </DrawerDescription>
          </DrawerHeader>
          <div
            ref={bodyRef}
            data-testid="task-management-scroll"
            className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-contain px-2 pb-[calc(1rem+env(safe-area-inset-bottom,0px))] pt-2"
          >
            {children}
          </div>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function Choice({
  children,
  icon,
  nested,
  disabled,
  destructive,
  onClick,
  testId,
}: {
  children: ReactNode;
  icon?: ReactNode;
  nested?: boolean;
  disabled?: boolean;
  destructive?: boolean;
  onClick?: () => void;
  testId?: string;
}) {
  return (
    <button
      type="button"
      data-testid={testId}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50",
        destructive && "text-destructive",
      )}
    >
      {icon}
      <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{children}</span>
      {nested && <IconChevronRight className="size-4 shrink-0" />}
    </button>
  );
}

function StepChoices({
  steps,
  currentStepId,
  disabled,
  onSelect,
}: {
  steps: TaskMoveStep[];
  currentStepId?: string;
  disabled?: boolean;
  onSelect: (stepId: string) => void;
}) {
  const { t } = useTranslation();
  return steps.map((step) => (
    <Choice
      key={step.id}
      testId={`task-context-step-${step.id}`}
      disabled={disabled || step.id === currentStepId}
      onClick={() => onSelect(step.id)}
    >
      <span className="flex min-w-0 items-center gap-2">
        <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{step.title}</span>
        {step.id === currentStepId && (
          <span className="shrink-0 text-xs text-muted-foreground">{t("task:current2")}</span>
        )}
        {stepHasAutoStart(step) && (
          <span className="shrink-0 text-xs text-muted-foreground">{t("task:autoStart")}</span>
        )}
      </span>
    </Choice>
  ));
}

function PriorityChoices({ task, disabled, onPriority }: TaskManagementDrawerProps) {
  const { t } = useTranslation();
  return TASK_PRIORITY_TOKENS.map((priority) => (
    <Choice
      key={priority}
      testId={`task-context-priority-${priority}`}
      disabled={disabled}
      onClick={() => onPriority(priority)}
    >
      <span className="flex items-center justify-between gap-2">
        {t(TASK_PRIORITY_LABEL_KEYS[priority])}
        {task.priority === priority && (
          <span className="text-xs text-muted-foreground">{t("kanban:current")}</span>
        )}
      </span>
    </Choice>
  ));
}

function DrawerChoices({
  page,
  setPage,
  props,
}: {
  page: Page;
  setPage: (page: Page) => void;
  props: TaskManagementDrawerProps;
}) {
  const { t } = useTranslation();
  const { task, workflows, stepsByWorkflowId, disabled, onMove, linkActions, closeMenu } = props;
  const { currentSteps, targets } = taskMoveOptions(task.workflowId, workflows, stepsByWorkflowId);
  const links = taskLinkMenuOptions(linkActions);
  const plugins = useTaskPluginLinkActions(task.id, task.repositoryLinks ?? []);
  if (page === "priority") return <PriorityChoices {...props} />;
  if (page === "steps")
    return (
      <StepChoices
        steps={currentSteps}
        currentStepId={task.workflowStepId}
        disabled={disabled}
        onSelect={(stepId) => {
          if (task.workflowId) onMove(task.workflowId, stepId);
        }}
      />
    );
  if (typeof page === "object")
    return (
      <StepChoices
        steps={stepsByWorkflowId[page.workflowId] ?? []}
        disabled={disabled}
        onSelect={(stepId) => onMove(page.workflowId, stepId)}
      />
    );
  if (page === "workflows")
    return targets.map((workflow) => (
      <Choice
        key={workflow.id}
        testId={`task-context-workflow-${workflow.id}`}
        nested
        disabled={disabled || !stepsByWorkflowId[workflow.id]?.length}
        onClick={() => setPage({ workflowId: workflow.id })}
      >
        {workflow.name}
        {!stepsByWorkflowId[workflow.id]?.length && (
          <span className="ml-2 text-xs">{t("task:noSteps")}</span>
        )}
      </Choice>
    ));
  if (page === "links")
    return (
      <>
        {links.map(({ id, labelKey, Icon, onSelect }) => (
          <Choice
            key={id}
            disabled={disabled}
            icon={<Icon className="size-4 shrink-0" />}
            onClick={onSelect}
          >
            {t(labelKey)}
          </Choice>
        ))}
        {plugins.map((action) => (
          <Choice
            key={action.id}
            disabled={disabled}
            icon={createElement(resolvePluginIcon(action.icon), { className: "size-4 shrink-0" })}
            onClick={() => {
              closeMenu();
              queueMicrotask(action.onSelect);
            }}
          >
            {action.label}
          </Choice>
        ))}
      </>
    );
  return (
    <RootChoices
      props={props}
      setPage={setPage}
      hasLinks={links.length > 0 || plugins.length > 0}
    />
  );
}

function RootChoices({
  props,
  setPage,
  hasLinks,
}: {
  props: TaskManagementDrawerProps;
  setPage: (page: Page) => void;
  hasLinks: boolean;
}) {
  const { t } = useTranslation();
  const { task, workflows, stepsByWorkflowId, disabled, onArchive, onDelete } = props;
  const { canMove, canSend } = taskMoveOptions(task.workflowId, workflows, stepsByWorkflowId);
  return (
    <>
      <Choice
        testId="task-management-page-priority"
        nested
        disabled={disabled}
        icon={<IconFlag className="size-4" />}
        onClick={() => setPage("priority")}
      >
        {t("kanban:priority")}
      </Choice>
      {canMove && (
        <Choice
          testId="task-management-page-steps"
          nested
          disabled={disabled}
          icon={<IconArrowRight className="size-4" />}
          onClick={() => setPage("steps")}
        >
          {t("task:moveTo")}
        </Choice>
      )}
      {canSend && (
        <Choice
          testId="task-management-page-workflows"
          nested
          disabled={disabled}
          icon={<IconLogicBuffer className="size-4" />}
          onClick={() => setPage("workflows")}
        >
          {t("task:sendToWorkflow")}
        </Choice>
      )}
      {hasLinks && (
        <Choice
          testId="task-management-page-links"
          nested
          disabled={disabled}
          icon={<IconLink className="size-4" />}
          onClick={() => setPage("links")}
        >
          {t("task:link")}
        </Choice>
      )}
      <Choice disabled={disabled} icon={<IconArchive className="size-4" />} onClick={onArchive}>
        {t("task:archive")}
      </Choice>
      <hr className="my-1 border-border" />
      <Choice
        destructive
        disabled={disabled}
        icon={<IconTrash className="size-4" />}
        onClick={onDelete}
      >
        {t("task:delete")}
      </Choice>
    </>
  );
}

export function TaskManagementDrawer(props: TaskManagementDrawerProps) {
  const { t } = useTranslation();
  const [page, setPage] = useState<Page>("root");
  const [focusChoiceId, setFocusChoiceId] = useState<string>();
  const navigate = (next: Page) => {
    setFocusChoiceId(undefined);
    setPage(next);
  };
  const back = () => {
    setFocusChoiceId(
      typeof page === "object"
        ? `task-context-workflow-${page.workflowId}`
        : `task-management-page-${page}`,
    );
    setPage(typeof page === "object" ? "workflows" : "root");
  };
  const title =
    typeof page === "object"
      ? (props.workflows.find((workflow) => workflow.id === page.workflowId)?.name ??
        t("task:sendToWorkflow"))
      : t(
          {
            root: "task:taskActions",
            priority: "kanban:priority",
            steps: "task:moveTo",
            workflows: "task:sendToWorkflow",
            links: "task:link",
          }[page],
        );
  return (
    <TaskManagementSheet
      title={title}
      taskTitle={props.task.title}
      onClose={props.closeMenu}
      onCloseAutoFocus={props.onCloseAutoFocus}
      focusChoiceId={focusChoiceId}
      onBack={page === "root" ? undefined : back}
    >
      <DrawerChoices page={page} setPage={navigate} props={props} />
    </TaskManagementSheet>
  );
}
