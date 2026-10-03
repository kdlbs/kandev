"use client";

import {
  Fragment,
  memo,
  useCallback,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type PointerEvent,
  type Ref,
} from "react";
import { useTranslation } from "react-i18next";
import {
  IconArrowBigRightLines,
  IconCheck,
  IconChevronDown,
  IconLogicBuffer,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTaskCreateDialogPopoverContainer } from "@/hooks/use-task-create-dialog-popover-container";
import { useWorkflowOptionPreviews } from "@/hooks/use-workflow-option-previews";
import type { WorkflowOptionPreview } from "@/hooks/use-workflow-option-previews";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import type { AgentProfileOption } from "@/lib/state/slices";
import { AgentLogo } from "@/components/agent-logo";
import type { TaskCreateLaunchPreview } from "@/components/task-create-dialog-launch-preview";
import { controlSizingClassName } from "@kandev/ui/control-sizing";

type StepItem = {
  id: string;
  title: string;
  color: string;
  position: number;
  agent_profile_id?: string;
  is_start_step?: boolean;
};

function InlineSteps({
  steps,
  agentProfiles,
}: {
  steps: StepItem[];
  agentProfiles: AgentProfileOption[];
}) {
  const { t } = useTranslation();
  if (steps.length === 0) return null;
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1 text-xs text-muted-foreground">
      {steps.map((s, i) => {
        const stepProfile = s.agent_profile_id
          ? agentProfiles.find((p) => p.id === s.agent_profile_id)
          : null;
        return (
          <Fragment key={s.id}>
            {i > 0 && <span className="text-muted-foreground/40">{"\u2192"}</span>}
            <span className="flex min-w-0 items-center gap-1">
              <span
                className="h-1.5 w-1.5 rounded-full shrink-0"
                style={{ backgroundColor: s.color || "hsl(var(--muted-foreground))" }}
              />
              <span className="min-w-0 wrap-anywhere" data-testid="workflow-option-step-name">
                {s.title}
              </span>
              {s.is_start_step && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-[10px] text-muted-foreground/60 leading-none">*</span>
                    </TooltipTrigger>
                    <TooltipContent>{t("workflows:startStep")}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
              {stepProfile && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span data-testid="step-agent-logo">
                        <AgentLogo
                          agentName={stepProfile.agent_name}
                          size={12}
                          className="shrink-0"
                        />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{stepProfile.label}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
            </span>
          </Fragment>
        );
      })}
    </div>
  );
}

type WorkflowSelectorRowProps = {
  workflows: Array<{
    id: string;
    name: string;
    description?: string | null;
    agent_profile_id?: string;
  }>;
  snapshots: Record<string, WorkflowSnapshotData>;
  selectedWorkflowId: string | null;
  onWorkflowChange: (workflowId: string) => void;
  agentProfiles: AgentProfileOption[];
  launchPreview?: TaskCreateLaunchPreview | null;
  previewWorkspaceId?: string | null;
  clearLabel?: string;
  placeholder?: string;
};

function WorkflowSelectorTrigger({
  selectedWorkflow,
  placeholder,
  triggerRef,
}: {
  selectedWorkflow: WorkflowSelectorRowProps["workflows"][number] | undefined;
  placeholder?: string;
  triggerRef: Ref<HTMLButtonElement>;
}) {
  const { t } = useTranslation();
  return (
    <PopoverTrigger asChild>
      <Button
        ref={triggerRef}
        type="button"
        variant="ghost"
        className={`${controlSizingClassName("standard")} w-auto min-w-0 max-w-full justify-between cursor-pointer`}
        data-testid="workflow-selector-trigger"
      >
        <IconLogicBuffer className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 truncate">
          {selectedWorkflow?.name ?? placeholder ?? t("workflows:selectWorkflow")}
        </span>
        <IconChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
      </Button>
    </PopoverTrigger>
  );
}

function LaunchDestinationInfo() {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const label = t("task:launchDestinationHelpLabel");
  const description = t("task:launchDestinationHelp");
  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
      aria-label={label}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? drawerOpen : undefined}
      data-testid="task-create-launch-step-info"
    >
      <IconArrowBigRightLines
        className="h-3.5 w-3.5"
        aria-hidden="true"
        data-testid="task-create-launch-step-arrow"
      />
    </Button>
  );

  if (usesTouchDrawer) {
    return (
      <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent data-testid="task-create-launch-step-help-drawer">
          <DrawerHeader>
            <DrawerTitle>{label}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>{trigger}</TooltipTrigger>
      <TooltipContent className="max-w-[320px] text-xs leading-relaxed">
        {description}
      </TooltipContent>
    </Tooltip>
  );
}

function LaunchDestinationLabel({ stepName }: { stepName: string }) {
  return (
    <span
      className="min-w-0 max-w-[45vw] shrink truncate text-xs text-muted-foreground"
      data-testid="task-create-launch-step"
    >
      {stepName}
    </span>
  );
}

type WorkflowItem = WorkflowSelectorRowProps["workflows"][number];

function getOptionSteps(
  taskCreatePreviewMode: boolean,
  preview: WorkflowOptionPreview | undefined,
  snapshot: WorkflowSnapshotData | undefined,
): StepItem[] {
  if (taskCreatePreviewMode) {
    return preview?.status === "success" ? preview.steps : [];
  }
  return snapshot ? [...snapshot.steps].sort((a, b) => a.position - b.position) : [];
}

type WorkflowPreviewStatusKey =
  | "workflows:loadingSteps"
  | "workflows:failedToLoadWorkflowSteps"
  | "workflows:noStepsInThisWorkflow";

function getWorkflowPreviewStatusKey(
  preview: WorkflowOptionPreview | undefined,
): WorkflowPreviewStatusKey | null {
  if (!preview || (preview.status === "success" && preview.steps.length > 0)) return null;
  if (preview.status === "loading") return "workflows:loadingSteps";
  if (preview.status === "error") return "workflows:failedToLoadWorkflowSteps";
  return "workflows:noStepsInThisWorkflow";
}

function useWorkflowRetryFocus(
  workflowId: string,
  previewStatus: WorkflowOptionPreview["status"] | undefined,
  onRetry: (workflowId: string) => void,
) {
  const [retrying, setRetrying] = useState(false);
  const optionButtonRef = useRef<HTMLButtonElement>(null);
  const retryEnteredLoading = useRef(false);

  const handleRetryPointerDown = useCallback((event: PointerEvent<HTMLButtonElement>) => {
    if (event.pointerType === "touch") {
      event.currentTarget.focus({ preventScroll: true });
    }
  }, []);

  useLayoutEffect(() => {
    if (!retrying) return;
    if (previewStatus === "loading") {
      retryEnteredLoading.current = true;
      return;
    }
    if (previewStatus === "success") {
      setRetrying(false);
      optionButtonRef.current?.focus({ preventScroll: true });
      retryEnteredLoading.current = false;
      return;
    }
    if (!retryEnteredLoading.current) return;
    setRetrying(false);
    retryEnteredLoading.current = false;
  }, [previewStatus, retrying]);

  const handleRetry = useCallback(
    (event: MouseEvent<HTMLButtonElement>) => {
      if (retrying) return;
      if (document.activeElement !== event.currentTarget) {
        optionButtonRef.current?.focus({ preventScroll: true });
      }
      retryEnteredLoading.current = false;
      setRetrying(true);
      onRetry(workflowId);
    },
    [onRetry, retrying, workflowId],
  );

  return {
    handleRetryPointerDown,
    handleRetry,
    optionButtonRef,
    retrying,
    showRetry: previewStatus === "error" || retrying,
  };
}

function WorkflowPreviewRetryButton({
  workflowId,
  retrying,
  onPointerDown,
  onClick,
}: {
  workflowId: string;
  retrying: boolean;
  onPointerDown: (event: PointerEvent<HTMLButtonElement>) => void;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="ghost"
      className={controlSizingClassName(
        "standard",
        "[@media(pointer:coarse)]:min-h-[48px] shrink-0 px-2 text-xs",
      )}
      aria-disabled={retrying || undefined}
      data-testid={`workflow-preview-retry-${workflowId}`}
      onPointerDown={onPointerDown}
      onClick={onClick}
    >
      {t("common:retryPreview")}
    </Button>
  );
}

function WorkflowOption({
  workflow,
  snapshot,
  preview,
  previewStatusKey,
  isSelected,
  taskCreatePreviewMode,
  agentProfiles,
  onWorkflowChange,
  onClose,
  onRetry,
}: {
  workflow: WorkflowItem;
  snapshot: WorkflowSnapshotData | undefined;
  preview: WorkflowOptionPreview | undefined;
  previewStatusKey: WorkflowPreviewStatusKey | null;
  isSelected: boolean;
  taskCreatePreviewMode: boolean;
  agentProfiles: AgentProfileOption[];
  onWorkflowChange: (workflowId: string) => void;
  onClose: () => void;
  onRetry: (workflowId: string) => void;
}) {
  const steps = getOptionSteps(taskCreatePreviewMode, preview, snapshot);
  const previewStatus = preview?.status;
  const retry = useWorkflowRetryFocus(workflow.id, previewStatus, onRetry);
  const workflowProfile = workflow.agent_profile_id
    ? agentProfiles.find((profile) => profile.id === workflow.agent_profile_id)
    : null;

  return (
    <div
      className="flex min-w-0 items-start gap-1 rounded-sm pr-1"
      data-testid={`workflow-option-${workflow.id}`}
    >
      <button
        type="button"
        ref={retry.optionButtonRef}
        aria-label={workflow.name}
        aria-pressed={isSelected}
        data-testid={`workflow-option-select-${workflow.id}`}
        onClick={() => {
          onWorkflowChange(workflow.id);
          onClose();
        }}
        className="relative flex min-h-[48px] min-w-0 flex-1 cursor-pointer flex-col gap-1 rounded-sm px-2 py-1.5 pr-8 text-left transition-colors hover:bg-muted"
      >
        <div className="flex min-w-0 items-center gap-2">
          <IconLogicBuffer className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <span className="min-w-0 break-words text-sm">{workflow.name}</span>
          {workflowProfile && (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span data-testid="workflow-agent-logo">
                    <AgentLogo
                      agentName={workflowProfile.agent_name}
                      size={14}
                      className="shrink-0"
                    />
                  </span>
                </TooltipTrigger>
                <TooltipContent>{workflowProfile.label}</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
        </div>
        {steps.length > 0 && (
          <div
            className="min-w-0 pl-[calc(0.875rem+0.5rem)]"
            data-testid={`workflow-option-steps-${workflow.id}`}
          >
            <InlineSteps steps={steps} agentProfiles={agentProfiles} />
          </div>
        )}
        <WorkflowPreviewStatus statusKey={taskCreatePreviewMode ? previewStatusKey : null} />
        {isSelected && <IconCheck className="absolute right-2 top-3 h-4 w-4" aria-hidden="true" />}
      </button>
      {retry.showRetry && (
        <WorkflowPreviewRetryButton
          workflowId={workflow.id}
          retrying={retry.retrying}
          onPointerDown={retry.handleRetryPointerDown}
          onClick={retry.handleRetry}
        />
      )}
    </div>
  );
}

type WorkflowSelectorOptionListProps = {
  workflows: WorkflowSelectorRowProps["workflows"];
  snapshots: WorkflowSelectorRowProps["snapshots"];
  previews: Record<string, WorkflowOptionPreview>;
  selectedWorkflowId: string | null;
  taskCreatePreviewMode: boolean;
  agentProfiles: WorkflowSelectorRowProps["agentProfiles"];
  clearLabel?: string;
  onWorkflowChange: WorkflowSelectorRowProps["onWorkflowChange"];
  onRetry: (workflowId: string) => void;
  onClose: () => void;
  triggerRef: { current: HTMLButtonElement | null };
  restoreFocusOnCloseRef: { current: boolean };
  portalContainer: HTMLElement | null;
};

function WorkflowSelectorOptionList({
  workflows,
  snapshots,
  previews,
  selectedWorkflowId,
  taskCreatePreviewMode,
  agentProfiles,
  clearLabel,
  onWorkflowChange,
  onRetry,
  onClose,
  triggerRef,
  restoreFocusOnCloseRef,
  portalContainer,
}: WorkflowSelectorOptionListProps) {
  const { t } = useTranslation();
  const previewStatusAnnouncement = workflows
    .map((workflow) => {
      const statusKey = getWorkflowPreviewStatusKey(previews[workflow.id]);
      return statusKey
        ? t("workflows:workflowPreviewStatusAnnouncement", {
            workflowName: workflow.name,
            status: t(statusKey),
          })
        : null;
    })
    .filter((message): message is string => message !== null)
    .join(" ");
  const closeAndRestoreFocus = () => {
    restoreFocusOnCloseRef.current = true;
    onClose();
  };

  return (
    <PopoverContent
      className="max-h-[var(--radix-popover-content-available-height)] w-[min(30rem,calc(100vw-1rem))] min-w-0 max-w-[calc(100vw-1rem)] gap-0 overflow-hidden p-1"
      align="start"
      collisionPadding={8}
      data-testid="workflow-selector-popover"
      portalContainer={portalContainer}
      onKeyDownCapture={(event) => {
        if (event.key === "Escape") restoreFocusOnCloseRef.current = true;
      }}
      onCloseAutoFocus={(event) => {
        if (!restoreFocusOnCloseRef.current) return;
        restoreFocusOnCloseRef.current = false;
        event.preventDefault();
        triggerRef.current?.focus({ preventScroll: true });
        requestAnimationFrame(() => {
          if (triggerRef.current?.isConnected) {
            triggerRef.current.focus({ preventScroll: true });
          }
        });
      }}
    >
      <div className="shrink-0 border-b px-2 py-1.5 text-xs text-muted-foreground">
        {t("workflows:workflow")}
      </div>
      <div
        className="sr-only"
        role="status"
        aria-atomic="true"
        data-testid="workflow-preview-status-announcement"
      >
        {previewStatusAnnouncement}
      </div>
      <div
        className="min-h-0 max-h-[min(32rem,calc(var(--radix-popover-content-available-height)-3rem))] overflow-y-auto overflow-x-hidden overscroll-contain"
        data-testid="workflow-selector-option-list"
      >
        {clearLabel ? (
          <button
            type="button"
            onClick={() => {
              onWorkflowChange("");
              closeAndRestoreFocus();
            }}
            className="min-h-11 w-full rounded-sm px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            {clearLabel}
          </button>
        ) : null}
        {workflows.map((workflow) => (
          <WorkflowOption
            key={workflow.id}
            workflow={workflow}
            snapshot={snapshots[workflow.id]}
            preview={previews[workflow.id]}
            previewStatusKey={getWorkflowPreviewStatusKey(previews[workflow.id])}
            isSelected={workflow.id === selectedWorkflowId}
            taskCreatePreviewMode={taskCreatePreviewMode}
            agentProfiles={agentProfiles}
            onWorkflowChange={onWorkflowChange}
            onClose={closeAndRestoreFocus}
            onRetry={onRetry}
          />
        ))}
      </div>
    </PopoverContent>
  );
}

export const WorkflowSelectorRow = memo(function WorkflowSelectorRow({
  workflows,
  snapshots,
  selectedWorkflowId,
  onWorkflowChange,
  agentProfiles,
  launchPreview,
  previewWorkspaceId,
  clearLabel,
  placeholder,
}: WorkflowSelectorRowProps) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const restoreFocusOnCloseRef = useRef(false);
  const portalContainer = useTaskCreateDialogPopoverContainer();
  const taskCreatePreviewMode = previewWorkspaceId !== undefined;
  const { previews, retry } = useWorkflowOptionPreviews(
    taskCreatePreviewMode ? previewWorkspaceId : null,
    open && taskCreatePreviewMode,
    taskCreatePreviewMode ? workflows.map((workflow) => workflow.id) : [],
  );

  const selectedWorkflow = useMemo(
    () => workflows.find((w) => w.id === selectedWorkflowId),
    [workflows, selectedWorkflowId],
  );

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen) restoreFocusOnCloseRef.current = false;
        setOpen(nextOpen);
      }}
    >
      <div className="flex min-w-0 items-center gap-2" data-testid="workflow-selector-row">
        <WorkflowSelectorTrigger
          selectedWorkflow={selectedWorkflow}
          placeholder={placeholder}
          triggerRef={triggerRef}
        />
        {launchPreview && (
          <>
            <LaunchDestinationInfo />
            <LaunchDestinationLabel stepName={launchPreview.stepName} />
          </>
        )}
      </div>
      <WorkflowSelectorOptionList
        workflows={workflows}
        snapshots={snapshots}
        previews={previews}
        selectedWorkflowId={selectedWorkflowId}
        taskCreatePreviewMode={taskCreatePreviewMode}
        agentProfiles={agentProfiles}
        clearLabel={clearLabel}
        onWorkflowChange={onWorkflowChange}
        onRetry={retry}
        onClose={() => setOpen(false)}
        triggerRef={triggerRef}
        restoreFocusOnCloseRef={restoreFocusOnCloseRef}
        portalContainer={portalContainer}
      />
    </Popover>
  );
});

function WorkflowPreviewStatus({ statusKey }: { statusKey: WorkflowPreviewStatusKey | null }) {
  const { t } = useTranslation();
  if (!statusKey) return null;

  return (
    <span className="pl-[calc(0.875rem+0.5rem)] text-xs text-muted-foreground" aria-hidden="true">
      {t(statusKey)}
    </span>
  );
}
