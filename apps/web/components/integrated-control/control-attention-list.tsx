import { useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { numberCellClass } from "./control-table";
import Link from "@/components/routing/app-link";
import { controlHref } from "@/lib/navigation/integrated-control";
import { linkToTask } from "@/lib/links";
import { countAttention, type AttentionItem, type AttentionKind } from "./control-attention";
import { ControlFilterChips } from "./control-filter-chips";
import { ControlStatusPill, pillLabelKey } from "./control-status-pill";
import { durationSince, occurredTime, sessionStateLabel, taskStateLabel } from "./overview-format";
import { ATTENTION_ANCHOR } from "./control-summary-head";

type Filter = Exclude<AttentionKind, "config"> | "all";
const FILTERS: Filter[] = ["all", "question", "stalled", "failed", "repeat"];

function repeatReason(t: TFunction, item: AttentionItem) {
  return t("sidebar:controlSummary.attention.reasonRepeat", {
    moves: item.repeat?.count ?? 0,
    start: occurredTime(item.repeat?.start),
    end: occurredTime(item.repeat?.end),
  });
}

function stalledReason(t: TFunction, item: AttentionItem) {
  if (item.scope === "workspace")
    return t("sidebar:controlSummary.attention.reasonWorkspaceStalled", {
      inProgress: item.figures?.inProgress ?? 0,
      queued: item.figures?.queued ?? 0,
    });
  return t("sidebar:controlSummary.attention.reasonTaskStalled", {
    queued: item.task?.queued_messages ?? 0,
  });
}

const REASONS: Record<AttentionKind, (t: TFunction, item: AttentionItem) => string> = {
  question: (t, item) =>
    t("sidebar:controlSummary.attention.reasonQuestion", {
      answerable: item.inbox?.answerable ?? 0,
      earlier: item.inbox?.earlier ?? 0,
      recovery: item.inbox?.recovery ?? 0,
    }),
  repeat: repeatReason,
  stalled: stalledReason,
  failed: (t, item) =>
    t("sidebar:controlSummary.attention.reasonFailed", {
      state: taskStateLabel(t, item.task?.state) || t("common:unknown"),
    }),
  config: (t, item) =>
    t("sidebar:controlSummary.attention.reasonConfig", { workspaces: item.workspaceCount }),
};

/** Why a row is listed; a task row also carries its primary session state. */
function reason(t: TFunction, item: AttentionItem): string {
  const text = REASONS[item.kind](t, item);
  const state = item.task?.session_state;
  return state && item.kind !== "failed" ? `${text} · ${sessionStateLabel(t, state)}` : text;
}

function target(item: AttentionItem, firstWorkspaceId?: string): { href: string; key: string } {
  if (item.kind === "question")
    return { href: controlHref("inbox"), key: "sidebar:controlSummary.attention.openInbox" };
  const taskId = item.task?.task_id ?? item.repeat?.taskId;
  if (taskId)
    return {
      href: linkToTask(
        taskId,
        item.task?.session_id ? { sessionId: item.task.session_id } : undefined,
      ),
      key: "sidebar:controlSummary.attention.open",
    };
  if (item.scope === "workspace" && item.workspaceId)
    return {
      href: controlHref("workspaces", item.workspaceId),
      key: "sidebar:controlSummary.attention.viewTasks",
    };
  return {
    href: firstWorkspaceId
      ? `/workspaces/${encodeURIComponent(firstWorkspaceId)}/coordinator`
      : "/settings",
    key: "sidebar:controlSummary.attention.settings",
  };
}

function subject(t: TFunction, item: AttentionItem): string {
  if (item.scope === "global") return t("sidebar:integratedControl.coordinators");
  if (item.kind === "question")
    return t("sidebar:controlSummary.attention.questionSubject", {
      count:
        (item.inbox?.answerable ?? 0) + (item.inbox?.earlier ?? 0) + (item.inbox?.recovery ?? 0),
    });
  if (item.scope === "workspace") return t("sidebar:controlSummary.attention.wholeWorkspace");
  return item.task?.title || item.repeat?.title || t("common:unknown");
}

export function ControlAttentionList({
  items,
  names,
}: {
  items: AttentionItem[];
  names: Record<string, string>;
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<Filter>("all");
  const counts = countAttention(items);
  const visible = filter === "all" ? items : items.filter((item) => item.kind === filter);
  const firstWorkspaceId = Object.keys(names)[0];
  return (
    <section
      id={ATTENTION_ANCHOR}
      className="scroll-mt-4 space-y-2"
      data-testid="control-attention"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-sm font-semibold">
          {t("sidebar:controlSummary.attention.title", { count: items.length })}
        </h2>
        <ControlFilterChips
          chips={FILTERS.map((value) => ({
            value,
            count: counts[value],
            label: value === "all" ? t("office:all") : t(pillLabelKey(value)),
          }))}
          selected={filter}
          onSelect={setFilter}
          testId="control-attention-filter"
        />
      </div>
      {visible.length === 0 ? (
        <p className="rounded-lg border px-4 py-3 text-sm text-muted-foreground">
          {t("sidebar:controlSummary.attention.empty")}
        </p>
      ) : (
        <div className="rounded-lg border">
          <Table className="min-w-[48rem] table-fixed">
            <AttentionTableHead />
            <TableBody>
              {visible.map((item, index) => {
                const link = target(item, firstWorkspaceId);
                const since = item.task?.updated_at ?? item.inbox?.oldest;
                return (
                  <TableRow
                    key={`${item.kind}:${item.task?.task_id ?? item.repeat?.taskId ?? item.workspaceId ?? index}`}
                    data-testid="control-attention-row"
                  >
                    <TableCell>
                      <ControlStatusPill state={item.kind} />
                    </TableCell>
                    <TableCell className="truncate">
                      {item.workspaceId
                        ? (names[item.workspaceId] ?? item.workspaceId)
                        : t("sidebar:controlSummary.attention.allWorkspaces")}
                    </TableCell>
                    <TableCell className="whitespace-normal break-words">
                      {subject(t, item)}
                    </TableCell>
                    <TableCell className="whitespace-normal text-xs text-muted-foreground">
                      {reason(t, item)}
                    </TableCell>
                    <TableCell
                      className={numberCellClass(undefined, "text-xs text-muted-foreground")}
                    >
                      {since ? durationSince(since) : "-"}
                    </TableCell>
                    <TableCell>
                      <Link
                        className="text-sm text-primary underline-offset-4 hover:underline"
                        href={link.href}
                      >
                        {t(link.key)}
                      </Link>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  );
}

/** Fixed column widths keep the duration column lined up across filters. */
function AttentionTableHead() {
  const { t } = useTranslation();
  return (
    <>
      <colgroup>
        <col className="w-28" />
        <col className="w-36" />
        <col />
        <col />
        <col className="w-20" />
        <col className="w-28" />
      </colgroup>
      <TableHeader>
        <TableRow>
          <TableHead>{t("sidebar:controlSummary.column.state")}</TableHead>
          <TableHead>{t("sidebar:controlSummary.column.workspace")}</TableHead>
          <TableHead>{t("sidebar:controlSummary.column.task")}</TableHead>
          <TableHead>{t("sidebar:controlSummary.column.reason")}</TableHead>
          <TableHead className="text-right">
            {t("sidebar:controlSummary.column.duration")}
          </TableHead>
          <TableHead>
            <span className="sr-only">{t("sidebar:controlSummary.attention.open")}</span>
          </TableHead>
        </TableRow>
      </TableHeader>
    </>
  );
}
