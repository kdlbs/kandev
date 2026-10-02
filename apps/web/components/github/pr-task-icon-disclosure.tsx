"use client";

import {
  forwardRef,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEventHandler,
  type FocusEventHandler,
  type MouseEventHandler,
  type PointerEventHandler,
  type ReactNode,
  type Ref,
} from "react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { TaskPR } from "@/lib/types/github";
import type { TaskPRTooltipHydrationStatus } from "@/hooks/domains/github/use-task-pr-tooltip-hydration";
import { useChangeRequestTaskTooltipState } from "@/components/integrations/use-change-request-task-tooltip-state";
import type { TaskPRAutomationSummary, TaskPRInfo } from "./pr-task-automation";
import {
  PRTaskStatusSummary,
  type PRTaskStatusSummaryData,
  type StaleWorkflowAttentionPR,
} from "./pr-task-status-summary";
import { PRStatusGlyph } from "./pr-status-glyph";
export { AutomationIndicatorDots } from "./pr-status-glyph";

export type PRTaskIconDisclosureProps = {
  taskId: string;
  prInfo?: TaskPRInfo;
  disclosurePRNumber?: number;
  disclosurePRRepository?: string;
  disclosurePRCount?: number;
  prs: TaskPR[];
  hasFullData: boolean;
  singlePR: TaskPR | null;
  readyToMerge: boolean;
  allReadyToMerge: boolean;
  displayState: string | undefined;
  displayCount: number;
  iconColor: string;
  ariaLabel: string;
  icon: ReactNode;
  content: ReactNode;
};

export function createPRTaskIconDisclosureProps({
  taskId,
  prInfo,
  disclosurePRNumber,
  disclosurePRRepository,
  disclosurePRCount,
  prs,
  hasFullData,
  singlePR,
  readyToMerge,
  allReadyToMerge,
  displayState,
  displayCount,
  iconColor,
  ariaLabel,
  automation,
  hasMergeConflicts,
  hasWorkflowApprovalRequired,
  summaries,
  staleWorkflowPRs,
  hydrationStatus,
}: Omit<PRTaskIconDisclosureProps, "icon" | "content"> & {
  automation: TaskPRAutomationSummary;
  hasMergeConflicts: boolean;
  hasWorkflowApprovalRequired: boolean;
  summaries: PRTaskStatusSummaryData[];
  staleWorkflowPRs: StaleWorkflowAttentionPR[];
  hydrationStatus: TaskPRTooltipHydrationStatus;
}): PRTaskIconDisclosureProps {
  return {
    taskId,
    prInfo,
    disclosurePRNumber,
    disclosurePRRepository,
    disclosurePRCount,
    prs,
    hasFullData,
    singlePR,
    readyToMerge,
    allReadyToMerge,
    displayState,
    displayCount,
    iconColor,
    ariaLabel,
    icon: (
      <PRTaskIconGlyph
        automation={automation}
        hasMergeConflicts={hasMergeConflicts}
        hasWorkflowApprovalRequired={hasWorkflowApprovalRequired}
      />
    ),
    content: getTaskPRIconDisclosureContent({
      hasFullData,
      summaries,
      staleWorkflowPRs,
      automation,
      hydrationStatus,
      hasWorkflowApprovalRequired,
    }),
  };
}

function getTaskPRIconDisclosureContent({
  hasFullData,
  summaries,
  staleWorkflowPRs,
  automation,
  hydrationStatus,
  hasWorkflowApprovalRequired,
}: {
  hasFullData: boolean;
  summaries: PRTaskStatusSummaryData[];
  staleWorkflowPRs: StaleWorkflowAttentionPR[];
  automation: TaskPRAutomationSummary;
  hydrationStatus: TaskPRTooltipHydrationStatus;
  hasWorkflowApprovalRequired: boolean;
}) {
  if (hasFullData) {
    return (
      <>
        <PRTaskStatusSummary summaries={summaries} staleWorkflowPRs={staleWorkflowPRs} />
        <TaskPRAutomationDetails summary={automation} />
      </>
    );
  }
  return (
    <>
      <CompactPRTooltipContent
        status={hydrationStatus}
        workflowApprovalRequired={hasWorkflowApprovalRequired}
      />
      <TaskPRAutomationDetails summary={automation} status={hydrationStatus} />
    </>
  );
}

export function PRTaskIconGlyph({
  automation,
  hasMergeConflicts,
  hasWorkflowApprovalRequired,
}: {
  automation: TaskPRAutomationSummary;
  hasMergeConflicts: boolean;
  hasWorkflowApprovalRequired: boolean;
}) {
  return (
    <PRStatusGlyph
      hasMergeConflicts={hasMergeConflicts}
      hasWorkflowApprovalRequired={hasWorkflowApprovalRequired}
      autoFixEnabled={automation.autoFixEnabled}
      autoMergeEnabled={automation.autoMergeEnabled}
    />
  );
}

export function PRTaskIconDrawer({
  open,
  onOpenChange,
  t,
  ...props
}: PRTaskIconDisclosureProps & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  const {
    prs,
    prInfo,
    singlePR,
    content,
    disclosurePRNumber,
    disclosurePRRepository,
    disclosurePRCount,
  } = props;
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerTrigger asChild>
        <PRTaskIconTrigger
          {...props}
          touch
          open={open}
          onClick={(event) => event.stopPropagation()}
        />
      </DrawerTrigger>
      <DrawerContent
        data-testid={`pr-task-automation-drawer-${props.taskId}`}
        className="max-h-[80dvh] flex flex-col"
      >
        <DrawerHeader className="shrink-0 border-b py-2">
          <DrawerTitle className="text-sm">
            {getDrawerTitle({
              t,
              prs,
              prInfo,
              singlePR,
              disclosurePRNumber,
              disclosurePRRepository,
              disclosurePRCount,
            })}
          </DrawerTitle>
          <DrawerDescription className="sr-only">
            {t("github:pullRequestCiStatusReviewsAnd")}
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 flex-1 overflow-y-auto p-3" data-vaul-no-drag>
          {content}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

export function PRTaskIconTooltip({
  tooltip,
  ...props
}: PRTaskIconDisclosureProps & {
  tooltip: ReturnType<typeof useChangeRequestTaskTooltipState>;
}) {
  const { t } = useTranslation();
  const triggerRef = useRef<HTMLElement>(null);
  const scrollBodyRef = useRef<HTMLDivElement>(null);
  const [tooltipDescription, setTooltipDescription] = useState(props.ariaLabel);
  useLayoutEffect(() => {
    const scrollBody = scrollBodyRef.current;
    if (!scrollBody) return;
    const description = (scrollBody.innerText || scrollBody.textContent || "")
      .replace(/\s+/g, " ")
      .trim();
    if (description) setTooltipDescription(description);
  }, [props.ariaLabel, props.content, tooltip.open]);
  const onEscapeKeyDown = (event: Event) => {
    if (tooltip.onEscapeKeyDown(event)) triggerRef.current?.focus();
  };
  const onTriggerKeyDown: KeyboardEventHandler<HTMLSpanElement> = (event) => {
    if (event.key !== "Tab" || event.shiftKey || !tooltip.open) return;
    event.preventDefault();
    scrollBodyRef.current?.focus();
  };

  return (
    <Tooltip open={tooltip.open}>
      <TooltipTrigger asChild>
        <PRTaskIconTrigger
          {...props}
          ref={triggerRef}
          onPointerEnter={tooltip.onPointerEnter}
          onPointerLeave={tooltip.onPointerLeave}
          onFocus={tooltip.onFocus}
          onBlur={tooltip.onBlur}
          onKeyDown={onTriggerKeyDown}
        />
      </TooltipTrigger>
      <TooltipContent
        sideOffset={6}
        onEscapeKeyDown={onEscapeKeyDown}
        onPointerEnter={tooltip.onContentPointerEnter}
        onPointerLeave={tooltip.onContentPointerLeave}
        onFocus={tooltip.onContentFocus}
        onBlur={tooltip.onContentBlur}
        aria-label={tooltipDescription}
        className="pointer-events-auto flex w-80 max-w-[calc(100vw-1rem)] flex-col p-3"
        style={{
          maxHeight: "min(var(--radix-tooltip-content-available-height), calc(100dvh - 1rem))",
        }}
      >
        <div
          ref={scrollBodyRef}
          data-testid="pr-task-summary-scroll-body"
          tabIndex={0}
          role="region"
          aria-label={t("github:pullRequestCiStatusReviewsAnd")}
          className="min-h-0 flex-1 overflow-y-auto overscroll-contain rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {props.content}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

type PRTaskIconTriggerProps = PRTaskIconDisclosureProps & {
  touch?: boolean;
  open?: boolean;
  onClick?: MouseEventHandler<HTMLButtonElement>;
  onPointerEnter?: PointerEventHandler<HTMLSpanElement>;
  onPointerLeave?: PointerEventHandler<HTMLSpanElement>;
  onFocus?: FocusEventHandler<HTMLSpanElement>;
  onBlur?: FocusEventHandler<HTMLSpanElement>;
  onKeyDown?: KeyboardEventHandler<HTMLSpanElement>;
};

export const PRTaskIconTrigger = forwardRef<HTMLElement, PRTaskIconTriggerProps>(
  function PRTaskIconTrigger(
    {
      touch = false,
      open = false,
      onClick,
      taskId,
      prInfo: _prInfo,
      disclosurePRNumber: _disclosurePRNumber,
      disclosurePRRepository: _disclosurePRRepository,
      disclosurePRCount: _disclosurePRCount,
      prs,
      hasFullData,
      singlePR: _singlePR,
      readyToMerge,
      allReadyToMerge,
      displayState,
      displayCount,
      iconColor,
      ariaLabel,
      icon,
      content: _content,
      onPointerEnter,
      onPointerLeave,
      onFocus,
      onBlur,
      onKeyDown,
      ...triggerAttributes
    },
    ref: Ref<HTMLElement>,
  ) {
    const commonAttributes = {
      "data-testid": `pr-task-icon-${taskId}`,
      "data-pr-state": displayState,
      "data-pr-count": displayCount,
      "data-pr-ready-to-merge": hasFullData
        ? String(prs.length === 1 ? readyToMerge : allReadyToMerge)
        : undefined,
      "aria-label": ariaLabel,
    };
    const hasMultiplePRs = displayCount > 1;
    const contents = (
      <span className="inline-flex items-center gap-0.5">
        {icon}
        {hasMultiplePRs ? (
          <span className="text-[9px] font-semibold leading-none tabular-nums">{displayCount}</span>
        ) : null}
      </span>
    );
    if (touch) {
      return (
        <button
          ref={ref as Ref<HTMLButtonElement>}
          type="button"
          {...triggerAttributes}
          {...commonAttributes}
          aria-haspopup="dialog"
          aria-expanded={open}
          className={cn(
            "relative inline-flex h-3.5 w-3.5 shrink-0 cursor-pointer items-center justify-center [@media(pointer:coarse)]:after:absolute [@media(pointer:coarse)]:after:-inset-[15px] [@media(pointer:coarse)]:after:content-['']",
            iconColor,
          )}
          onPointerDown={(event) => event.stopPropagation()}
          onClick={onClick}
        >
          {contents}
        </button>
      );
    }
    return (
      <span
        ref={ref as Ref<HTMLSpanElement>}
        {...triggerAttributes}
        {...commonAttributes}
        role="img"
        tabIndex={0}
        className={cn("inline-flex shrink-0 items-center", hasMultiplePRs && "gap-0.5", iconColor)}
        onPointerEnter={onPointerEnter}
        onPointerLeave={onPointerLeave}
        onFocus={onFocus}
        onBlur={onBlur}
        onKeyDown={onKeyDown}
      >
        {contents}
      </span>
    );
  },
);

function getDrawerTitle({
  t,
  prs,
  prInfo,
  singlePR,
  disclosurePRNumber,
  disclosurePRRepository,
  disclosurePRCount,
}: {
  t: ReturnType<typeof useTranslation>["t"];
  prs: TaskPR[];
  prInfo?: TaskPRInfo;
  singlePR: TaskPR | null;
  disclosurePRNumber?: number;
  disclosurePRRepository?: string;
  disclosurePRCount?: number;
}) {
  const count = disclosurePRCount ?? prs.length;
  if (count > 1) return t("github:pullRequestCount", { count });
  if (disclosurePRRepository && disclosurePRNumber) {
    return t("github:prTaskStatusRepositoryNumber", {
      repository: disclosurePRRepository,
      number: disclosurePRNumber,
    });
  }
  return t("github:pullRequestStatus", {
    number: disclosurePRNumber ?? singlePR?.pr_number ?? prInfo?.number,
  });
}

export function TaskPRAutomationDetails({
  summary,
  status,
}: {
  summary: TaskPRAutomationSummary;
  status?: TaskPRTooltipHydrationStatus;
}) {
  const { t } = useTranslation();
  const hasAutomation = summary.autoFixEnabled || summary.autoMergeEnabled;
  if (!hasAutomation && summary.details.length === 0) return null;
  const detailContent =
    summary.details.length > 0 ? (
      <div className="mt-1 space-y-1">
        {summary.details.map((detail) => (
          <div
            key={`${detail.repository ?? ""}-${detail.number}`}
            className="flex flex-wrap items-center gap-x-2 gap-y-1"
          >
            <span className="font-medium">
              {detail.repository
                ? t("github:prTaskStatusRepositoryNumber", {
                    repository: detail.repository,
                    number: detail.number,
                  })
                : t("github:prTaskStatusNumber", { number: detail.number })}
            </span>
            {detail.autoFixEnabled && (
              <span className="text-yellow-500">{t("github:autoFix")}</span>
            )}
            {detail.autoMergeEnabled && (
              <span className="text-purple-400">{t("github:autoMerge")}</span>
            )}
          </div>
        ))}
      </div>
    ) : null;
  const loadingContent =
    status === "loading" || status === "idle" ? (
      <p className="mt-1 text-muted-foreground">{t("github:taskPrDetailsLoading")}</p>
    ) : null;
  return (
    <section
      data-testid="pr-task-automation-details"
      className="mt-2 border-t border-border/60 pt-2 text-xs"
    >
      <h4 className="font-medium text-foreground">{t("github:automation")}</h4>
      {detailContent ?? loadingContent}
    </section>
  );
}

function CompactWorkflowApprovalLabel({ required }: { required: boolean }) {
  const { t } = useTranslation();
  if (!required) return null;
  return (
    <div className="text-sm font-medium text-foreground">
      {t("github:workflowAwaitingApproval")}
    </div>
  );
}

export function CompactPRTooltipContent({
  status,
  workflowApprovalRequired = false,
}: {
  status: TaskPRTooltipHydrationStatus;
  workflowApprovalRequired?: boolean;
}) {
  const { t } = useTranslation();
  if (status === "loading" || status === "idle") {
    return (
      <>
        <CompactWorkflowApprovalLabel required={workflowApprovalRequired} />
        <span data-testid="pr-task-tooltip-loading" className="text-sm text-muted-foreground">
          {t("github:taskPrDetailsLoading")}
        </span>
      </>
    );
  }
  if (status === "unavailable") {
    return (
      <>
        <CompactWorkflowApprovalLabel required={workflowApprovalRequired} />
        <span data-testid="pr-task-tooltip-unavailable" className="text-sm text-muted-foreground">
          {t("github:taskPrDetailsUnavailable")}
        </span>
      </>
    );
  }
  return null;
}
