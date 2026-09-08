"use client";

import { IconGitPullRequest } from "@tabler/icons-react";
import {
  forwardRef,
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

export type PRTaskIconDisclosureProps = {
  taskId: string;
  prInfo?: TaskPRInfo;
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

export function AutomationIndicatorDots({
  autoFixEnabled,
  autoMergeEnabled,
}: Pick<TaskPRAutomationSummary, "autoFixEnabled" | "autoMergeEnabled">) {
  return (
    <>
      {autoFixEnabled && (
        <span
          data-testid="pr-task-automation-auto-fix"
          className="absolute left-0 top-0 h-1.5 w-1.5 rounded-full bg-yellow-400 ring-1 ring-background"
          aria-hidden="true"
        />
      )}
      {autoMergeEnabled && (
        <span
          data-testid="pr-task-automation-auto-merge"
          className="absolute right-0 top-0 h-1.5 w-1.5 rounded-full bg-purple-500 ring-1 ring-background"
          aria-hidden="true"
        />
      )}
    </>
  );
}

export function PRTaskIconGlyph({ automation }: { automation: TaskPRAutomationSummary }) {
  return (
    <span className="relative inline-flex h-3.5 w-3.5 shrink-0">
      <IconGitPullRequest aria-hidden="true" className="h-3.5 w-3.5" />
      <AutomationIndicatorDots
        autoFixEnabled={automation.autoFixEnabled}
        autoMergeEnabled={automation.autoMergeEnabled}
      />
    </span>
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
  const { prs, prInfo, singlePR, content } = props;
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
            {prs.length > 1
              ? t("github:pullRequestCount", { count: prs.length })
              : t("github:pullRequestStatus", { number: singlePR?.pr_number ?? prInfo?.number })}
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
  return (
    <Tooltip open={tooltip.open}>
      <TooltipTrigger asChild>
        <PRTaskIconTrigger
          {...props}
          onPointerEnter={tooltip.onPointerEnter}
          onPointerLeave={tooltip.onPointerLeave}
          onFocus={tooltip.onFocus}
          onBlur={tooltip.onBlur}
        />
      </TooltipTrigger>
      <TooltipContent
        sideOffset={6}
        onEscapeKeyDown={tooltip.onEscapeKeyDown}
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        {props.content}
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
};

export const PRTaskIconTrigger = forwardRef<HTMLElement, PRTaskIconTriggerProps>(
  function PRTaskIconTrigger(
    {
      touch = false,
      open = false,
      onClick,
      taskId,
      prInfo: _prInfo,
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
    const contents = (
      <span className="inline-flex items-center gap-0.5">
        {icon}
        {prs.length > 1 ? (
          <span className="text-[9px] font-semibold leading-none tabular-nums">{prs.length}</span>
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
        className={cn("inline-flex shrink-0 items-center", prs.length > 1 && "gap-0.5", iconColor)}
        onPointerEnter={onPointerEnter}
        onPointerLeave={onPointerLeave}
        onFocus={onFocus}
        onBlur={onBlur}
      >
        {contents}
      </span>
    );
  },
);

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

export function CompactPRTooltipContent({ status }: { status: TaskPRTooltipHydrationStatus }) {
  const { t } = useTranslation();
  if (status === "loading" || status === "idle") {
    return (
      <span data-testid="pr-task-tooltip-loading" className="text-sm text-muted-foreground">
        {t("github:taskPrDetailsLoading")}
      </span>
    );
  }
  if (status === "unavailable") {
    return (
      <span data-testid="pr-task-tooltip-unavailable" className="text-sm text-muted-foreground">
        {t("github:taskPrDetailsUnavailable")}
      </span>
    );
  }
  return null;
}
