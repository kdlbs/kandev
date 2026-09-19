"use client";

import {
  IconSettings,
  IconTextWrap,
  IconLayoutColumns,
  IconLayoutRows,
  IconMessageForward,
  IconArrowsMaximize,
  IconRoute,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ReviewPRSelector } from "@/components/review/review-pr-selector";
import type { TaskPR } from "@/lib/types/github";
import { PanelHeaderBarSplit, PanelHeaderOverflowMenu } from "./panel-primitives";
import { useTranslation } from "react-i18next";

export type ChangesTopBarProps = {
  autoMarkOnScroll: boolean;
  splitView: boolean;
  wordWrap: boolean;
  totalCommentCount: number;
  reviewedCount: number;
  totalCount: number;
  progressPercent: number;
  setWordWrap: (v: boolean) => void;
  handleToggleSplitView: (v: boolean) => void;
  handleToggleAutoMark: (v: boolean) => void;
  handleFixComments: () => void;
  handleRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
  prs: TaskPR[];
  selectedPR: TaskPR | null;
  prDiffLoading: boolean;
  onSelectPR: (pr: TaskPR) => void;
};

function ChangesTopBarLeft({
  autoMarkOnScroll,
  totalCount,
  reviewedCount,
  progressPercent,
  handleToggleAutoMark,
}: Pick<
  ChangesTopBarProps,
  "autoMarkOnScroll" | "totalCount" | "reviewedCount" | "progressPercent" | "handleToggleAutoMark"
>) {
  const { t } = useTranslation();
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 min-w-6 px-1.5 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-w-11"
          >
            <IconSettings className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-64">
          <DropdownMenuItem
            className="cursor-pointer gap-2"
            onSelect={(e) => {
              e.preventDefault();
              handleToggleAutoMark(!autoMarkOnScroll);
            }}
          >
            <Checkbox checked={autoMarkOnScroll} className="pointer-events-none" />
            <span className="text-sm flex-1">{t("task:autoMarkReviewedOnScroll")}</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {totalCount > 0 && (
        <div className="flex items-center gap-2 min-w-0">
          <div className="w-20 h-1 rounded-full bg-muted overflow-hidden">
            <div
              className="h-full bg-indigo-500 rounded-full transition-all duration-300"
              style={{ width: `${progressPercent}%` }}
            />
          </div>
          <span className="text-[11px] text-muted-foreground whitespace-nowrap">
            {reviewedCount}/{totalCount} {t("task:reviewed")}
          </span>
        </div>
      )}
    </>
  );
}

function ReviewWalkthroughRequestButton({
  handleRequestWalkthrough,
  requestWalkthroughDisabled,
}: Pick<ChangesTopBarProps, "handleRequestWalkthrough" | "requestWalkthroughDisabled">) {
  const { t } = useTranslation();
  if (!handleRequestWalkthrough) return null;
  const tooltip = requestWalkthroughDisabled
    ? t("task:loadingChangedFiles")
    : t("task:walkMeThroughTheseChanges");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="inline-flex"
          tabIndex={requestWalkthroughDisabled ? 0 : undefined}
          aria-label={requestWalkthroughDisabled ? tooltip : undefined}
        >
          <Button
            size="sm"
            variant="ghost"
            className="h-6 min-w-6 px-1.5 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-w-11"
            aria-label={t("task:walkMeThroughTheseReviewChanges")}
            data-testid="review-request-walkthrough"
            disabled={requestWalkthroughDisabled}
            onClick={handleRequestWalkthrough}
          >
            <IconRoute className="h-3.5 w-3.5" />
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function ChangesTopBarRight({
  splitView,
  wordWrap,
  totalCommentCount,
  setWordWrap,
  handleToggleSplitView,
  handleFixComments,
  handleRequestWalkthrough,
  requestWalkthroughDisabled,
}: Pick<
  ChangesTopBarProps,
  | "splitView"
  | "wordWrap"
  | "totalCommentCount"
  | "setWordWrap"
  | "handleToggleSplitView"
  | "handleFixComments"
  | "handleRequestWalkthrough"
  | "requestWalkthroughDisabled"
>) {
  const { t } = useTranslation();
  return (
    <>
      <ReviewWalkthroughRequestButton
        handleRequestWalkthrough={handleRequestWalkthrough}
        requestWalkthroughDisabled={requestWalkthroughDisabled}
      />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 min-w-6 px-1.5 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-w-11"
            aria-label={t("task:expandReview")}
            onClick={() => window.dispatchEvent(new CustomEvent("open-review-dialog"))}
          >
            <IconArrowsMaximize className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("task:expandReview")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className={`h-6 min-w-6 px-1.5 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-w-11 ${wordWrap ? "bg-muted" : ""}`}
            onClick={() => setWordWrap(!wordWrap)}
          >
            <IconTextWrap className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("task:toggleWordWrap")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 min-w-6 px-1.5 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-w-11"
            onClick={() => handleToggleSplitView(!splitView)}
          >
            {splitView ? (
              <IconLayoutRows className="h-3.5 w-3.5" />
            ) : (
              <IconLayoutColumns className="h-3.5 w-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{splitView ? t("task:unifiedView") : t("task:splitView")}</TooltipContent>
      </Tooltip>
      {totalCommentCount > 0 && (
        <Button
          size="sm"
          variant="outline"
          className="min-h-6 text-xs cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={handleFixComments}
        >
          <IconMessageForward className="h-3.5 w-3.5" />
          {t("task:fix")}
          <span className="ml-0.5 rounded-full bg-blue-500/30 px-1 py-0 text-[10px] font-medium text-blue-600 dark:text-blue-400">
            {totalCommentCount}
          </span>
        </Button>
      )}
    </>
  );
}

function ChangesTopBarOverflowActions({
  splitView,
  wordWrap,
  totalCommentCount,
  setWordWrap,
  handleToggleSplitView,
  handleFixComments,
  handleRequestWalkthrough,
  requestWalkthroughDisabled,
}: Pick<
  ChangesTopBarProps,
  | "splitView"
  | "wordWrap"
  | "totalCommentCount"
  | "setWordWrap"
  | "handleToggleSplitView"
  | "handleFixComments"
  | "handleRequestWalkthrough"
  | "requestWalkthroughDisabled"
>) {
  const { t } = useTranslation();
  const walkthroughReason = requestWalkthroughDisabled
    ? t("task:loadingChangedFiles")
    : t("task:walkMeThroughTheseChanges");
  return (
    <PanelHeaderOverflowMenu label={t("common:showMoreActions")}>
      {handleRequestWalkthrough && (
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          disabled={requestWalkthroughDisabled}
          title={walkthroughReason}
          onSelect={handleRequestWalkthrough}
        >
          <IconRoute className="size-4" />
          {t("task:walkMeThroughTheseReviewChanges")}
        </DropdownMenuItem>
      )}
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => window.dispatchEvent(new CustomEvent("open-review-dialog"))}
      >
        <IconArrowsMaximize className="size-4" />
        {t("task:expandReview")}
      </DropdownMenuItem>
      <DropdownMenuItem className="cursor-pointer gap-2" onSelect={() => setWordWrap(!wordWrap)}>
        <IconTextWrap className="size-4" />
        {t("task:toggleWordWrap")}
      </DropdownMenuItem>
      <DropdownMenuItem
        className="cursor-pointer gap-2"
        onSelect={() => handleToggleSplitView(!splitView)}
      >
        {splitView ? (
          <IconLayoutRows className="size-4" />
        ) : (
          <IconLayoutColumns className="size-4" />
        )}
        {splitView ? t("task:unifiedView") : t("task:splitView")}
      </DropdownMenuItem>
      {totalCommentCount > 0 && (
        <DropdownMenuItem className="cursor-pointer gap-2" onSelect={handleFixComments}>
          <IconMessageForward className="size-4" />
          {t("task:fix")}
          <span className="ml-auto text-xs text-muted-foreground">{totalCommentCount}</span>
        </DropdownMenuItem>
      )}
    </PanelHeaderOverflowMenu>
  );
}

function MultiPRChangesTopBar({
  autoMarkOnScroll,
  splitView,
  wordWrap,
  totalCommentCount,
  reviewedCount,
  totalCount,
  progressPercent,
  setWordWrap,
  handleToggleSplitView,
  handleToggleAutoMark,
  handleFixComments,
  handleRequestWalkthrough,
  requestWalkthroughDisabled,
  prs,
  selectedPR,
  prDiffLoading,
  onSelectPR,
}: ChangesTopBarProps) {
  const selector = (
    <ReviewPRSelector
      prs={prs}
      selectedPR={selectedPR}
      loading={prDiffLoading}
      onSelectPR={onSelectPR}
      compact
      testIdPrefix="changes-review-pr-selector"
      className="min-w-0 max-w-full"
    />
  );

  return (
    <PanelHeaderBarSplit
      leftClassName="flex-1"
      left={
        <>
          <div className="order-first min-w-0 flex-1">{selector}</div>
          <ChangesTopBarLeft
            autoMarkOnScroll={autoMarkOnScroll}
            totalCount={totalCount}
            reviewedCount={reviewedCount}
            progressPercent={progressPercent}
            handleToggleAutoMark={handleToggleAutoMark}
          />
        </>
      }
      leftWhenOverflow={
        <div className="min-w-0 flex-1 [&>button]:w-full [&>button]:min-w-0">{selector}</div>
      }
      right={
        <ChangesTopBarRight
          splitView={splitView}
          wordWrap={wordWrap}
          totalCommentCount={totalCommentCount}
          setWordWrap={setWordWrap}
          handleToggleSplitView={handleToggleSplitView}
          handleFixComments={handleFixComments}
          handleRequestWalkthrough={handleRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      overflow={
        <ChangesTopBarOverflowActions
          splitView={splitView}
          wordWrap={wordWrap}
          totalCommentCount={totalCommentCount}
          setWordWrap={setWordWrap}
          handleToggleSplitView={handleToggleSplitView}
          handleFixComments={handleFixComments}
          handleRequestWalkthrough={handleRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      overflowAt={520}
    />
  );
}

function SinglePRChangesTopBar({
  autoMarkOnScroll,
  splitView,
  wordWrap,
  totalCommentCount,
  reviewedCount,
  totalCount,
  progressPercent,
  setWordWrap,
  handleToggleSplitView,
  handleToggleAutoMark,
  handleFixComments,
  handleRequestWalkthrough,
  requestWalkthroughDisabled,
}: ChangesTopBarProps) {
  return (
    <PanelHeaderBarSplit
      left={
        <ChangesTopBarLeft
          autoMarkOnScroll={autoMarkOnScroll}
          totalCount={totalCount}
          reviewedCount={reviewedCount}
          progressPercent={progressPercent}
          handleToggleAutoMark={handleToggleAutoMark}
        />
      }
      right={
        <ChangesTopBarRight
          splitView={splitView}
          wordWrap={wordWrap}
          totalCommentCount={totalCommentCount}
          setWordWrap={setWordWrap}
          handleToggleSplitView={handleToggleSplitView}
          handleFixComments={handleFixComments}
          handleRequestWalkthrough={handleRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      overflow={
        <ChangesTopBarOverflowActions
          splitView={splitView}
          wordWrap={wordWrap}
          totalCommentCount={totalCommentCount}
          setWordWrap={setWordWrap}
          handleToggleSplitView={handleToggleSplitView}
          handleFixComments={handleFixComments}
          handleRequestWalkthrough={handleRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      overflowAt={520}
    />
  );
}

export function ChangesTopBar(props: ChangesTopBarProps) {
  if (props.prs.length > 1 && props.selectedPR) {
    return <MultiPRChangesTopBar {...props} />;
  }
  return <SinglePRChangesTopBar {...props} />;
}
