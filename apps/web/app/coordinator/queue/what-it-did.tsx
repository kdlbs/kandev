import { useCallback, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import {
  isActivityClass,
  type ActivityClass,
  type ActivityItem,
} from "@/lib/api/domains/coordinator-activity-api";
import type { AttentionTask } from "@/lib/coordinator/attention";
import { useActivity } from "@/hooks/domains/coordinator/use-activity";
import { useNowTick } from "../use-now-tick";
import { MESSAGE_TEXT_KEY } from "./activity-text";
import { UndoDialog } from "./undo-dialog";
import { WhatItDidFilter } from "./what-it-did-filter";
import { ACTIVITY_GRID, WhatItDidRow } from "./what-it-did-row";

export type WhatItDidProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  tasks: AttentionTask[];
  /** From the Queue's `useCoordinatorAttention` result. */
  tasksLoadedAt: number | undefined;
  tasksError: boolean;
  stepNameByWorkflowStep: ReadonlyMap<string, string>;
};

function useStickyTrue(condition: boolean): boolean {
  const [sticky, setSticky] = useState(false);
  if (condition && !sticky) setSticky(true);
  return sticky || condition;
}

function stepTitleById(byWorkflowStep: ReadonlyMap<string, string>): Map<string, string> {
  const map = new Map<string, string>();
  for (const [key, title] of byWorkflowStep) map.set(key.slice(key.indexOf(":") + 1), title);
  return map;
}

function ColumnHeaders() {
  const { t } = useTranslation();
  return (
    <div
      role="row"
      className={`text-muted-foreground hidden gap-3 border-b px-3 py-1.5 text-[10px] font-semibold tracking-[0.09em] uppercase md:grid ${ACTIVITY_GRID}`}
    >
      <div role="columnheader">{t("coordinator:activityColWhen")}</div>
      <div role="columnheader">{t("coordinator:activityColAction")}</div>
      <div role="columnheader">{t("coordinator:activityFilterLabel")}</div>
      <div role="columnheader">{t("coordinator:activityColAuthorised")}</div>
      <div role="columnheader">{t("coordinator:activityUndoAction")}</div>
    </div>
  );
}

function ListStatus({
  status,
  isEmpty,
  filtered,
  retry,
}: {
  status: "loading" | "loaded" | "failed";
  isEmpty: boolean;
  filtered: boolean;
  retry: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {status === "loading" && (
        <p role="status" className="text-muted-foreground text-sm" data-testid="activity-loading">
          {t("coordinator:activityLoading")}
        </p>
      )}
      {status === "failed" && (
        <div className="flex items-center gap-2 text-sm" data-testid="activity-load-failed">
          <span>{t("coordinator:activityLoadFailed")}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="cursor-pointer"
            onClick={retry}
          >
            {t("coordinator:activityRetry")}
          </Button>
        </div>
      )}
      {status === "loaded" && isEmpty && (
        <p className="text-muted-foreground text-sm" data-testid="activity-empty">
          {filtered ? t("coordinator:activityFilteredEmpty") : t("coordinator:activityEmpty")}
        </p>
      )}
    </>
  );
}

function LoadMore({
  disabled,
  failed,
  onClick,
}: {
  disabled: boolean;
  failed: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-center gap-1">
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="cursor-pointer"
        disabled={disabled}
        onClick={onClick}
        data-testid="activity-load-more"
      >
        {t("coordinator:activityLoadMore")}
      </Button>
      {failed && (
        <p role="status" className="text-muted-foreground text-xs">
          {t("coordinator:activityLoadMoreFailed")}
        </p>
      )}
    </div>
  );
}

/**
 * The Queue's What it did section: newest first, a class filter kept in the
 * address, authorisation text, Undo for managers and the undone state
 * (docs/specs/coordinator/system-design/activity-log.md#what-it-did-ui).
 */
// eslint-disable-next-line max-lines-per-function -- one section owns the list states, the filter address and the undo dialog
export function WhatItDid(props: WhatItDidProps) {
  const { workspaceId, coordinatorId, canManage, tasks, stepNameByWorkflowStep } = props;
  const { t } = useTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const rawClass = searchParams.get("class");
  const activityClass = isActivityClass(rawClass) ? rawClass : undefined;
  const { snapshot, loadMore, retry, undo, resolvePerson } = useActivity({
    workspaceId,
    coordinatorId,
    activityClass,
  });
  const now = useNowTick();
  const trusted = useStickyTrue(props.tasksLoadedAt !== undefined && !props.tasksError);
  const heldTaskIds = useMemo(() => new Set(tasks.map((task) => task.id)), [tasks]);
  const stepNames = useMemo(() => stepTitleById(stepNameByWorkflowStep), [stepNameByWorkflowStep]);

  const [dialogItem, setDialogItem] = useState<ActivityItem | null>(null);
  const restoreRowId = useRef<string | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);

  const changeFilter = useCallback(
    (next: ActivityClass | undefined) => {
      const params = new URLSearchParams(searchParams);
      if (next) params.set("class", next);
      else params.delete("class");
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [router, pathname, searchParams],
  );

  const openDialog = (item: ActivityItem) => {
    restoreRowId.current = item.id;
    setDialogItem(item);
  };
  const restoreFocus = (event: Event) => {
    event.preventDefault();
    const rowButton = restoreRowId.current
      ? document.querySelector<HTMLElement>(`[data-testid="activity-undo-${restoreRowId.current}"]`)
      : null;
    (rowButton ?? heading.current)?.focus();
  };

  const { status, rows, nextCursor, loadMore: loadMoreState, rereading } = snapshot;
  const stepName = dialogItem?.from_step_id ? stepNames.get(dialogItem.from_step_id) : undefined;

  return (
    <section className="space-y-2" data-testid="what-it-did" aria-labelledby="activity-title">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2
          id="activity-title"
          ref={heading}
          tabIndex={-1}
          className="text-muted-foreground text-[10px] font-semibold tracking-[0.09em] uppercase outline-none"
        >
          {t("coordinator:activityTitle")}
        </h2>
        <WhatItDidFilter value={activityClass} onChange={changeFilter} />
      </div>
      {snapshot.notice && (
        <p role="status" className="text-muted-foreground text-xs" data-testid="activity-notice">
          {t(MESSAGE_TEXT_KEY[snapshot.notice.key])}
        </p>
      )}
      <ListStatus
        status={status}
        isEmpty={rows.length === 0}
        filtered={!!activityClass}
        retry={retry}
      />
      {status === "loaded" && rows.length > 0 && (
        <TooltipProvider>
          <div
            role="table"
            aria-label={t("coordinator:activityTitle")}
            className="border-border bg-card divide-border divide-y rounded-md border"
          >
            <ColumnHeaders />
            {rows.map((item) => (
              <WhatItDidRow
                key={item.id}
                item={item}
                now={now}
                canManage={canManage}
                busy={snapshot.busy.has(item.id)}
                message={snapshot.messages.get(item.id)}
                resolvePerson={resolvePerson}
                taskAvailability={{
                  held: item.target_task_id ? heldTaskIds.has(item.target_task_id) : false,
                  trusted,
                }}
                onUndo={openDialog}
              />
            ))}
          </div>
        </TooltipProvider>
      )}
      {status === "loaded" && nextCursor && (
        <LoadMore
          disabled={rereading || loadMoreState === "loading"}
          failed={loadMoreState === "failed"}
          onClick={loadMore}
        />
      )}
      <UndoDialog
        item={dialogItem}
        stepName={stepName}
        onConfirm={(item) => {
          setDialogItem(null);
          undo(item);
        }}
        onClose={() => setDialogItem(null)}
        onCloseAutoFocus={restoreFocus}
      />
    </section>
  );
}
