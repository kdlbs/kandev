"use client";

import { useCallback, useRef, useState, type ReactNode } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";
import {
  remoteContributionActionPolicy,
  type RemoteContributionRelation,
} from "@/hooks/domains/session/remote-contribution-relation";
import {
  contributionHistoryExplanationKey,
  useContributionHistoryExplanation,
  type ContributionHistoryExplanation,
  type ContributionHistoryExplanationTarget,
} from "@/hooks/domains/session/use-contribution-history-explanation";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { openExternalLink } from "@/lib/desktop/external-links";
import {
  type RemoteContributionResolutionTarget,
  type useRemoteContributionResolution,
  useRemoteContributionResolutionConfirmation,
} from "./use-remote-contribution-resolution";
import { RemoteContributionActionItems } from "./remote-contribution-action-items";
import { RemoteContributionResolutionDialog } from "./remote-contribution-resolution-dialog";
import { requestContributionComparison } from "./remote-contribution-comparison";

function isConfirmedLocalRebase(explanation: ContributionHistoryExplanation | null) {
  return explanation?.kind === "local_rebase" && explanation.reason === "matched_reflog";
}

function HistoryExplanationBody({
  status,
  explanation,
}: {
  status: "idle" | "loading" | "ready" | "unavailable";
  explanation: ContributionHistoryExplanation | null;
}) {
  const { t } = useTranslation();
  const localRebase = isConfirmedLocalRebase(explanation);
  return (
    <div className="space-y-2">
      <p className="font-normal leading-relaxed text-muted-foreground">
        {localRebase
          ? t("task:remoteContributionHistoryLocalRebase")
          : t("task:remoteContributionHistoryBody")}
      </p>
      {status === "loading" && (
        <p className="text-xs text-muted-foreground" data-testid="history-explanation-loading">
          {t("task:remoteContributionHistoryLoading")}
        </p>
      )}
      {localRebase && explanation && (
        <div className="space-y-1 text-xs text-muted-foreground" data-testid="history-counts">
          {typeof explanation.task_commit_count === "number" && (
            <p>
              {t("task:remoteContributionTaskCommits", {
                count: explanation.task_commit_count,
              })}
            </p>
          )}
          {typeof explanation.published_commit_count === "number" && (
            <p>
              {t("task:remoteContributionPublishedCommits", {
                count: explanation.published_commit_count,
              })}
            </p>
          )}
          {typeof explanation.new_base_commit_count === "number" && (
            <p>
              {t("task:remoteContributionNewBaseCommits", {
                count: explanation.new_base_commit_count,
              })}
            </p>
          )}
        </div>
      )}
      {localRebase && (
        <p className="text-xs text-muted-foreground">
          {t("task:remoteContributionHistoryGuidance")}
        </p>
      )}
    </div>
  );
}

type HistoryMenuBodyProps = {
  status: "idle" | "loading" | "ready" | "unavailable";
  explanation: ContributionHistoryExplanation | null;
  disabled: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  compareDisabled: boolean;
  descriptionMode: "tooltip" | "inline";
  surface: "menu" | "drawer";
  onCompare: () => void;
  onReplace: () => void;
  onUse: () => void;
  onView?: () => void;
  prNumber?: number;
  onCloseAutoFocus: (event: Event) => void;
};

type HistoryMenuActionsProps = Pick<
  HistoryMenuBodyProps,
  | "disabled"
  | "replaceDisabled"
  | "useDisabled"
  | "compareDisabled"
  | "descriptionMode"
  | "surface"
  | "onCompare"
  | "onReplace"
  | "onUse"
  | "onView"
  | "prNumber"
>;

function HistoryMenuActions({
  disabled,
  replaceDisabled,
  useDisabled,
  compareDisabled,
  descriptionMode,
  surface,
  onCompare,
  onReplace,
  onUse,
  onView,
  prNumber,
}: HistoryMenuActionsProps) {
  return (
    <RemoteContributionActionItems
      disabled={disabled}
      replaceDisabled={replaceDisabled}
      useDisabled={useDisabled}
      compareDisabled={compareDisabled}
      onCompareVersions={onCompare}
      onReplaceContribution={onReplace}
      onUseContribution={onUse}
      onViewPRVersion={onView}
      descriptionMode={descriptionMode}
      surface={surface}
      testIdPrefix="header"
      prNumber={prNumber}
      viewLabelKey="task:openPROnGitHub"
      replaceLabelKey="task:publishTaskVersion"
      useLabelKey="task:restorePublishedPRVersion"
      replaceDescriptionKey="task:remoteContributionPublishDescription"
      useDescriptionKey="task:remoteContributionRestoreDescription"
    />
  );
}

function HistoryMenuBody({
  status,
  explanation,
  disabled,
  replaceDisabled,
  useDisabled,
  compareDisabled,
  descriptionMode,
  surface,
  onCompare,
  onReplace,
  onUse,
  onView,
  prNumber,
  onCloseAutoFocus,
}: HistoryMenuBodyProps) {
  const { t } = useTranslation();
  const actions = (
    <HistoryMenuActions
      disabled={disabled}
      replaceDisabled={replaceDisabled}
      useDisabled={useDisabled}
      compareDisabled={compareDisabled}
      onCompare={onCompare}
      onReplace={onReplace}
      onUse={onUse}
      onView={onView}
      descriptionMode={descriptionMode}
      surface={surface}
      prNumber={prNumber}
    />
  );
  if (surface === "drawer") {
    return (
      <div
        className="flex min-h-0 flex-1 flex-col overflow-hidden"
        data-testid="header-remote-contribution-menu"
      >
        <DrawerHeader className="flex-row items-center justify-between gap-2 border-b border-border/70 pb-3 text-left">
          <DrawerTitle className="min-w-0 text-sm">
            {t("task:remoteContributionHistoryTitle")}
          </DrawerTitle>
          <DrawerClose asChild>
            <Button
              type="button"
              variant="ghost"
              className="min-h-11 shrink-0 px-3"
              data-testid="header-remote-contribution-close"
            >
              {t("task:close")}
            </Button>
          </DrawerClose>
          <DrawerDescription className="sr-only">
            {t("task:remoteContributionHistoryBody")}
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain px-4 py-3 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]">
          <HistoryExplanationBody status={status} explanation={explanation} />
          <div className="space-y-1 border-t border-border/70 pt-2">{actions}</div>
        </div>
      </div>
    );
  }
  return (
    <DropdownMenuContent
      align="end"
      className="w-80"
      data-testid="header-remote-contribution-menu"
      onCloseAutoFocus={onCloseAutoFocus}
    >
      <DropdownMenuLabel className="whitespace-normal px-2 py-2">
        <div className="flex items-start gap-2">
          <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" />
          <div className="min-w-0">
            <p className="font-medium text-foreground">
              {t("task:remoteContributionHistoryTitle")}
            </p>
            <HistoryExplanationBody status={status} explanation={explanation} />
          </div>
        </div>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      {actions}
    </DropdownMenuContent>
  );
}

type RemoteContributionHeaderActionsProps = {
  relation?: RemoteContributionRelation;
  resolution?: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget?: RemoteContributionResolutionTarget | null;
  contributionHistoryTarget?: ContributionHistoryExplanationTarget | null;
  prUrl?: string;
  prNumber?: number;
};

function useRemoteContributionMenu({
  comparisonKey,
  resolution,
  resolutionTarget,
  policy,
  prUrl,
}: {
  comparisonKey: string | null;
  resolution?: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget?: RemoteContributionResolutionTarget | null;
  policy: ReturnType<typeof remoteContributionActionPolicy> | null;
  prUrl?: string;
}) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const restoreFocusOnCloseRef = useRef(true);
  const closeMenu = useCallback((restoreFocus: boolean) => {
    restoreFocusOnCloseRef.current = restoreFocus;
    setOpen(false);
  }, []);
  const handleCloseAutoFocus = useCallback((event: Event) => {
    event.preventDefault();
    if (restoreFocusOnCloseRef.current) {
      queueMicrotask(() => triggerRef.current?.focus());
    }
    restoreFocusOnCloseRef.current = true;
  }, []);
  const handleOpenChange = useCallback((nextOpen: boolean) => {
    if (nextOpen) restoreFocusOnCloseRef.current = true;
    setOpen(nextOpen);
  }, []);
  const handleCompare = useCallback(() => {
    if (comparisonKey) requestContributionComparison(comparisonKey);
    closeMenu(false);
  }, [closeMenu, comparisonKey]);
  const handleReplace = useCallback(() => {
    if (!resolution || !resolutionTarget || policy?.replaceDisabled) return;
    closeMenu(true);
    resolution.requestReplace(resolutionTarget);
  }, [closeMenu, policy?.replaceDisabled, resolution, resolutionTarget]);
  const handleUse = useCallback(() => {
    if (!resolution || !resolutionTarget || policy?.useDisabled) return;
    closeMenu(true);
    resolution.requestUse(resolutionTarget);
  }, [closeMenu, policy?.useDisabled, resolution, resolutionTarget]);
  const handleView = useCallback(() => {
    closeMenu(true);
    if (prUrl) void openExternalLink(prUrl).catch(() => undefined);
  }, [closeMenu, prUrl]);

  return {
    open,
    triggerRef,
    handleCloseAutoFocus,
    handleOpenChange,
    handleCompare,
    handleReplace,
    handleUse,
    handleView,
  };
}

function RemoteContributionSurface({
  usesTouchDrawer,
  trigger,
  menuBody,
  open,
  onOpenChange,
  onCloseAutoFocus,
}: {
  usesTouchDrawer: boolean;
  trigger: ReactNode;
  menuBody: ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus: (event: Event) => void;
}) {
  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          onCloseAutoFocus={onCloseAutoFocus}
          className="min-h-0 overflow-hidden rounded-t-xl pb-[env(safe-area-inset-bottom,0px)]"
        >
          {menuBody}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      {menuBody}
    </DropdownMenu>
  );
}

export function RemoteContributionHeaderActions({
  relation,
  resolution,
  resolutionTarget,
  contributionHistoryTarget,
  prUrl,
  prNumber,
}: RemoteContributionHeaderActionsProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const confirmResolution = useRemoteContributionResolutionConfirmation(resolution);
  const policy = relation ? remoteContributionActionPolicy(relation) : null;
  const comparisonKey = contributionHistoryExplanationKey(contributionHistoryTarget ?? null);
  const menu = useRemoteContributionMenu({
    comparisonKey,
    resolution,
    resolutionTarget,
    policy,
    prUrl,
  });
  const history = useContributionHistoryExplanation(contributionHistoryTarget ?? null, menu.open);

  if (relation?.kind !== "diverged" || !resolution || !resolutionTarget || !policy) return null;
  const trigger = (
    <Button
      ref={menu.triggerRef}
      type="button"
      size="icon"
      variant="ghost"
      className="h-11 w-11 cursor-pointer text-yellow-600 hover:bg-yellow-500/10 hover:text-yellow-600 md:h-6 md:w-6 dark:text-yellow-400 dark:hover:text-yellow-300"
      aria-label={t("task:remoteContributionHistoryTitle")}
      title={t("task:remoteContributionHistoryTooltip")}
      data-testid="header-remote-contribution-warning"
      disabled={resolution.isLoading}
    >
      <IconAlertTriangle className="h-4 w-4" />
    </Button>
  );
  const menuBody = (
    <HistoryMenuBody
      status={history.status}
      explanation={history.explanation}
      disabled={resolution.isLoading}
      replaceDisabled={policy.replaceDisabled}
      useDisabled={policy.useDisabled}
      compareDisabled={!comparisonKey}
      descriptionMode={usesTouchDrawer ? "inline" : "tooltip"}
      surface={usesTouchDrawer ? "drawer" : "menu"}
      onCompare={menu.handleCompare}
      onReplace={menu.handleReplace}
      onUse={menu.handleUse}
      onView={prUrl ? menu.handleView : undefined}
      prNumber={prNumber}
      onCloseAutoFocus={menu.handleCloseAutoFocus}
    />
  );
  return (
    <div className="flex items-center gap-1">
      <RemoteContributionSurface
        usesTouchDrawer={usesTouchDrawer}
        trigger={trigger}
        menuBody={menuBody}
        open={menu.open}
        onOpenChange={menu.handleOpenChange}
        onCloseAutoFocus={menu.handleCloseAutoFocus}
      />
      {resolution.pending && (
        <RemoteContributionResolutionDialog
          open
          action={resolution.pending.action}
          repositoryName={resolutionTarget.repositoryName ?? ""}
          expectedRemoteHead={resolution.pending.expectedRemoteHead}
          isLoading={resolution.isLoading}
          errorKey={resolution.errorKey}
          onOpenChange={(isOpen) => {
            if (!isOpen) resolution.cancel();
          }}
          onConfirm={() => {
            void confirmResolution();
          }}
        />
      )}
    </div>
  );
}
