import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { ActivityRow } from "@/app/office/workspace/activity/activity-row";
import type { ControlAggregateRead } from "@/hooks/domains/coordinator/use-control-aggregate";
import { groupActivity } from "./control-aggregate-signals";

/**
 * The aggregate's merged recent Office activity across workspaces, newest
 * first, with consecutive repeats of the same action folded into one row.
 */
export function ControlGroupedHistory({ aggregate }: { aggregate: ControlAggregateRead }) {
  const { t } = useTranslation();
  const data = aggregate.data;
  const groups = useMemo(() => groupActivity(data?.recentActivity ?? []), [data]);
  const names = useMemo(
    () => Object.fromEntries((data?.workspaces ?? []).map((w) => [w.workspace_id, w.name])),
    [data],
  );
  return (
    <section className="rounded-lg border" data-testid="control-grouped-history">
      <h2 className="border-b px-4 py-3 font-medium">
        {t("sidebar:integratedControl.aggregate.historyTitle")}
      </h2>
      {!data && (
        <p
          role={aggregate.error ? "alert" : "status"}
          className={`px-4 py-3 text-sm ${aggregate.error ? "text-destructive" : "text-muted-foreground"}`}
        >
          {t(
            aggregate.error
              ? "sidebar:integratedControl.aggregate.loadError"
              : "sidebar:integratedControl.aggregate.loading",
          )}
        </p>
      )}
      {data && groups.length === 0 && (
        <p className="px-4 py-3 text-sm text-muted-foreground">
          {t("sidebar:integratedControl.aggregate.historyEmpty")}
        </p>
      )}
      {groups.length > 0 && (
        <ul className="divide-y">
          {groups.map((group) => (
            <li key={group.key} data-testid="control-history-group" data-count={group.count}>
              <div className="flex flex-wrap items-center gap-2 px-4 pt-2 text-xs text-muted-foreground">
                <span className="break-words">
                  {names[group.entry.workspaceId] ?? group.entry.workspaceId}
                </span>
                {group.count > 1 && (
                  <span
                    className="rounded border px-1.5 py-0.5"
                    data-testid="control-history-group-count"
                  >
                    {t("sidebar:integratedControl.aggregate.historyRepeated", {
                      count: group.count,
                    })}
                  </span>
                )}
              </div>
              <ActivityRow entry={group.entry} />
            </li>
          ))}
        </ul>
      )}
      {data && (
        <p className="border-t px-4 py-2 text-xs text-muted-foreground">
          {t("sidebar:integratedControl.aggregate.historyScope")}
        </p>
      )}
    </section>
  );
}
