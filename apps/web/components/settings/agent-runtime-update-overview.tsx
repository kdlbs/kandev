"use client";

import { useLayoutEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import type {
  RuntimeUpdateGroup,
  RuntimeUpdateManagerModel,
} from "@/lib/agents/runtime-update-manager";
import type { RuntimeRowListProps, RuntimeUpdateBulkProps } from "./agent-runtime-update-rows";
import { RuntimeRowList } from "./agent-runtime-update-rows";
import { RuntimeUpdateBatchResults } from "./agent-runtime-update-batch-results";
import { RuntimeReviewHeader, RuntimeReviewNotices } from "./agent-runtime-update-review";

export type RuntimeUpdateOverviewProps = {
  model: RuntimeUpdateManagerModel;
  isMobile: boolean;
  canManage: boolean;
  statusLoading: boolean;
  refreshFailed: boolean;
  refreshing: boolean;
  onRefresh: () => void;
  onOpenRuntime: (agentName: string, opener?: HTMLElement) => void;
  individualErrors: Record<string, string | undefined>;
  bulk?: RuntimeUpdateBulkProps;
  bulkBusy?: boolean;
  bulkError?: string;
  onClose: () => void;
  onReviewAgain: () => void;
  initialScrollTop: number;
  onScrollTopChange: (scrollTop: number) => void;
  openGroups: ReadonlySet<string>;
  onGroupOpenChange: (group: RuntimeUpdateGroup, open: boolean) => void;
  footerBefore?: ReactNode;
};

const GROUP_LABEL_KEYS: Record<Exclude<RuntimeUpdateGroup, "available">, string> = {
  up_to_date: "agents:runtimeGroupUpToDate",
  unknown: "agents:runtimeGroupUnknown",
  external: "agents:runtimeGroupExternal",
  unavailable: "agents:runtimeGroupUnavailable",
};

const SECONDARY_GROUPS: readonly Exclude<RuntimeUpdateGroup, "available">[] = [
  "up_to_date",
  "unknown",
  "external",
  "unavailable",
];

function groupLabel(
  t: (key: string, values?: Record<string, unknown>) => string,
  group: RuntimeUpdateGroup,
  count: number,
): string {
  if (group === "available") return t("agents:runtimeGroupAvailable", { count });
  return t(GROUP_LABEL_KEYS[group], { count });
}

function RuntimeOverviewStatus({
  statusLoading,
  refreshFailed,
  refreshing,
  empty,
  onRefresh,
}: {
  statusLoading: boolean;
  refreshFailed: boolean;
  refreshing: boolean;
  empty: boolean;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {statusLoading && empty && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
          <IconLoader2 className="size-4 animate-spin" />
          {t("agents:runtimeCheckingVersions")}
        </p>
      )}
      {refreshFailed && (
        <div
          className="space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-2"
          role="alert"
        >
          <p className="text-sm text-destructive">{t("agents:runtimeRefreshFailed")}</p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="cursor-pointer"
            disabled={statusLoading || refreshing}
            onClick={onRefresh}
          >
            {t("agents:runtimeRetryRefresh")}
          </Button>
        </div>
      )}
      {!statusLoading && empty && (
        <p className="text-sm text-muted-foreground" role="status">
          {t("agents:runtimeNoRegistrations")}
        </p>
      )}
    </>
  );
}

function RuntimeSecondaryGroups({
  model,
  openGroups,
  onGroupOpenChange,
  ...props
}: Omit<RuntimeRowListProps, "rows"> & {
  model: RuntimeUpdateManagerModel;
  openGroups: ReadonlySet<string>;
  onGroupOpenChange: (group: RuntimeUpdateGroup, open: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {SECONDARY_GROUPS.map((group) => {
        const groupRows = model.groups[group];
        if (groupRows.length === 0) return null;
        return (
          <details
            key={group}
            className="min-w-0 border-t pt-2"
            open={openGroups.has(group)}
            onToggle={(event) => onGroupOpenChange(group, event.currentTarget.open)}
          >
            <summary className="flex min-h-8 cursor-pointer items-center gap-1 py-1 text-sm font-medium max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
              <IconChevronRight className="size-4 shrink-0" aria-hidden="true" />
              {groupLabel(t, group, groupRows.length)}
            </summary>
            <RuntimeRowList rows={groupRows} {...props} />
          </details>
        );
      })}
    </>
  );
}

function RuntimeBatchOverview({
  bulk,
  onOpenRuntime,
  onReviewAgain,
}: {
  bulk: RuntimeUpdateBulkProps;
  onOpenRuntime: (agentName: string) => void;
  onReviewAgain: () => void;
}) {
  return (
    <RuntimeUpdateBatchResults
      batch={bulk.batch}
      onRetryFailed={bulk.onRetryFailed}
      onRefreshStatus={bulk.onRefreshStatus}
      onOpenRuntime={onOpenRuntime}
      onClose={() => undefined}
      onReviewAgain={onReviewAgain}
      showClose={false}
    />
  );
}

function RuntimeReviewOverview({
  model,
  bulk,
  bulkError,
  statusLoading,
  refreshFailed,
  refreshing,
  empty,
  onRefresh,
  listProps,
  openGroups,
  onGroupOpenChange,
}: {
  model: RuntimeUpdateManagerModel;
  bulk?: RuntimeUpdateBulkProps;
  bulkError?: string;
  statusLoading: boolean;
  refreshFailed: boolean;
  refreshing: boolean;
  empty: boolean;
  onRefresh: () => void;
  listProps: Omit<RuntimeRowListProps, "rows">;
  openGroups: ReadonlySet<string>;
  onGroupOpenChange: (group: RuntimeUpdateGroup, open: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <RuntimeOverviewStatus
        statusLoading={statusLoading}
        refreshFailed={refreshFailed}
        refreshing={refreshing}
        empty={empty}
        onRefresh={onRefresh}
      />
      {bulkError && (
        <p
          className="rounded-md border border-destructive/30 bg-destructive/5 p-2 text-sm text-destructive"
          role="alert"
          data-testid="runtime-bulk-error"
        >
          {bulkError}
        </p>
      )}
      {bulk?.batch.status === "review" && (
        <>
          <RuntimeReviewHeader bulk={bulk} loading={statusLoading || refreshing} />
          <RuntimeReviewNotices bulk={bulk} />
        </>
      )}
      {model.groups.available.length > 0 && (
        <div aria-labelledby="runtime-manager-available">
          <RuntimeRowList rows={model.groups.available} {...listProps} />
        </div>
      )}
      {model.individualOnlyCount > 0 && (
        <p className="text-xs text-muted-foreground" role="note">
          {t("agents:runtimeIndividualOnlyNote", { count: model.individualOnlyCount })}
        </p>
      )}
      <RuntimeSecondaryGroups
        model={model}
        {...listProps}
        openGroups={openGroups}
        onGroupOpenChange={onGroupOpenChange}
      />
    </>
  );
}

function RuntimeOverviewBody({
  model,
  isMobile,
  statusLoading,
  refreshFailed,
  refreshing,
  onRefresh,
  onOpenRuntime,
  individualErrors,
  bulk,
  bulkError,
  onReviewAgain,
  initialScrollTop,
  onScrollTopChange,
  openGroups,
  onGroupOpenChange,
}: Omit<RuntimeUpdateOverviewProps, "onClose" | "footerBefore"> & {
  onReviewAgain: () => void;
}) {
  const bodyRef = useRef<HTMLDivElement | null>(null);
  useLayoutEffect(() => {
    if (bodyRef.current) bodyRef.current.scrollTop = initialScrollTop;
  }, [initialScrollTop]);
  const { t } = useTranslation();
  const available = model.groups.available;
  const rows = [...available, ...SECONDARY_GROUPS.flatMap((group) => model.groups[group])];
  const listProps = {
    isMobile,
    onOpenRuntime,
    individualErrors,
    bulk,
  };
  const batchActive = bulk?.batch.status === "running" || bulk?.batch.status === "complete";
  return (
    <div
      ref={bodyRef}
      className="min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain px-4 py-3"
      data-testid="runtime-manager-body"
      onScroll={(event) => onScrollTopChange(event.currentTarget.scrollTop)}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        {batchActive ? (
          <h3 className="text-sm font-semibold">{t("agents:runtimeManagerTitle")}</h3>
        ) : (
          <h3 className="text-sm font-semibold" id="runtime-manager-available">
            {groupLabel(t, "available", available.length)}
          </h3>
        )}
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          disabled={statusLoading || refreshing}
          onClick={onRefresh}
          data-testid="runtime-manager-refresh"
        >
          {statusLoading || refreshing ? (
            <IconLoader2 className="mr-2 size-4 animate-spin" />
          ) : (
            <IconRefresh className="mr-2 size-4" />
          )}
          {t("agents:runtimeCheckForUpdates")}
        </Button>
      </div>
      {batchActive && bulk ? (
        <RuntimeBatchOverview
          bulk={bulk}
          onOpenRuntime={onOpenRuntime}
          onReviewAgain={onReviewAgain}
        />
      ) : (
        <RuntimeReviewOverview
          model={model}
          bulk={bulk}
          bulkError={bulkError}
          statusLoading={statusLoading}
          refreshFailed={refreshFailed}
          refreshing={refreshing}
          empty={rows.length === 0}
          onRefresh={onRefresh}
          listProps={listProps}
          openGroups={openGroups}
          onGroupOpenChange={onGroupOpenChange}
        />
      )}
    </div>
  );
}

function bulkApprovalState(
  props: RuntimeUpdateOverviewProps,
  t: (key: string, values?: Record<string, unknown>) => string,
) {
  const { bulk } = props;
  const eligibleKeys = new Set(props.model.bulkCandidates.map((row) => row.key));
  const selectedEligible = new Set(
    [...(bulk?.selection ?? [])].filter((key) => eligibleKeys.has(key)),
  );
  const allEligibleSelected =
    Boolean(bulk) && bulk!.reviewKeys.size > 0 && selectedEligible.size === bulk!.reviewKeys.size;
  return {
    eligibleKeys,
    canApprove:
      Boolean(bulk) &&
      bulk?.batch.status === "review" &&
      selectedEligible.size > 0 &&
      !bulk.reviewLoading &&
      !props.statusLoading &&
      !props.refreshing &&
      !props.refreshFailed &&
      !props.bulkBusy &&
      props.canManage,
    label: allEligibleSelected
      ? t("agents:runtimeUpdateAll", { count: selectedEligible.size })
      : t("agents:runtimeUpdateSelected", { count: selectedEligible.size }),
  };
}

export function RuntimeUpdateOverviewSurface(props: RuntimeUpdateOverviewProps) {
  const { t } = useTranslation();
  const { isMobile, onClose, footerBefore, bulk, onReviewAgain } = props;
  const body = <RuntimeOverviewBody {...props} onReviewAgain={onReviewAgain} />;
  const approval = bulkApprovalState(props, t);
  const footer = (
    <>
      {footerBefore}
      <div className="flex w-full flex-wrap items-center justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={onClose}
          data-testid="runtime-manager-close"
        >
          {t("common:close")}
        </Button>
        {bulk?.batch.status === "review" && (
          <Button
            type="button"
            className="cursor-pointer max-md:min-h-12 [@media(pointer:coarse)]:min-h-12"
            disabled={!approval.canApprove}
            onClick={() => bulk.onStart(approval.eligibleKeys)}
            data-testid="runtime-bulk-approve"
          >
            {approval.label}
          </Button>
        )}
      </div>
    </>
  );
  if (isMobile) {
    return (
      <Drawer open onOpenChange={(open) => !open && onClose()}>
        <DrawerContent
          className="flex max-h-[92dvh] flex-col overflow-hidden data-[vaul-drawer-direction=bottom]:max-h-[92dvh]"
          data-testid="runtime-manager-drawer"
        >
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>{t("agents:runtimeManagerTitle")}</DrawerTitle>
            <DrawerDescription>{t("agents:runtimeManagerDescription")}</DrawerDescription>
          </DrawerHeader>
          {body}
          <div className="shrink-0 space-y-2 border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {footer}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:min-w-[44rem] sm:max-w-3xl"
        data-testid="runtime-manager-dialog"
      >
        <DialogHeader className="shrink-0 px-4 pb-2 pt-3">
          <DialogTitle>{t("agents:runtimeManagerTitle")}</DialogTitle>
          <DialogDescription>{t("agents:runtimeManagerDescription")}</DialogDescription>
        </DialogHeader>
        {body}
        <div className="shrink-0 space-y-2 border-t px-4 py-3">{footer}</div>
      </DialogContent>
    </Dialog>
  );
}
