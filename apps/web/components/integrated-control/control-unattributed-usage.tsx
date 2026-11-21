import { useTranslation } from "react-i18next";
import type { ProfileExecutionUsage } from "@/lib/api/domains/provider-usage-api";

/** Unattributed reports are execution evidence, never fabricated provider accounts. */
export function ControlUnattributedUsage({ entries }: { entries: ProfileExecutionUsage[] }) {
  const { t } = useTranslation();
  if (!entries.length) return null;
  const rows = new Map<string, ProfileExecutionUsage[]>();
  for (const entry of entries) {
    const key = JSON.stringify([entry.report.model ?? "", entry.report.reason]);
    rows.set(key, [...(rows.get(key) ?? []), entry]);
  }
  return (
    <details className="rounded-lg border px-3 py-2" data-testid="control-unattributed-usage">
      <summary className="min-h-7 cursor-pointer [@media(pointer:coarse)]:min-h-11">
        <h2 className="inline text-sm font-medium">
          {t("sidebar:controlRefresh.unattributedTitle", { count: entries.length })}
        </h2>
      </summary>
      <p className="pt-1 text-xs text-muted-foreground">
        {t("sidebar:controlRefresh.unattributedScope")}
      </p>
      <div className="divide-y">
        {[...rows.entries()].map(([key, reports]) => (
          <details key={key} className="py-1 text-xs" data-testid="control-unattributed-model">
            <summary className="min-h-7 cursor-pointer [@media(pointer:coarse)]:min-h-11">
              <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="break-all font-medium">
                  {reports[0].report.model || t("common:unknown")}
                </span>
                <span>{t("sidebar:controlRestore.limit.unknown")}</span>
                <span className="break-all text-muted-foreground">
                  {reports[0].report.reason || t("sidebar:controlInspection.usageUnknown")}
                </span>
                <span className="tabular-nums">{reports.length}</span>
              </span>
            </summary>
            <ul className="space-y-1 break-all px-3 py-2 font-mono text-muted-foreground">
              {reports.map((entry) => (
                <li key={entry.execution_id}>
                  {entry.execution_id} · {entry.session_id || t("common:unknown")}
                </li>
              ))}
            </ul>
          </details>
        ))}
      </div>
    </details>
  );
}
