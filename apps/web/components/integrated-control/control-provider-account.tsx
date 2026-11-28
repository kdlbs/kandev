import { useTranslation } from "react-i18next";
import { Card } from "@kandev/ui/card";
import { formatDateTime } from "@/lib/i18n/formats";
import {
  latestModelUsage,
  observedLimitState,
  type ProviderAccountObservation,
} from "@/lib/control-ui/usage";

/** Account summary followed by its resolved models, as in the fork provider groups. */
export function ControlProviderAccount({ group }: { group: ProviderAccountObservation }) {
  const { t } = useTranslation();
  const unknown = t("common:unknown");
  const sample = group.sample;
  const date = (at?: string) =>
    at && Number.isFinite(Date.parse(at)) ? formatDateTime(at) : unknown;
  return (
    <Card className="space-y-3 p-4" data-testid="control-provider-account">
      <h2 className="break-words font-medium">{group.provider || unknown}</h2>
      <dl className="grid gap-x-4 gap-y-1 break-words text-sm sm:grid-cols-[auto_minmax(0,1fr)]">
        <dt>{t("agents:nativeUsage.account")}</dt>
        <dd>{group.account || unknown}</dd>
        <dt>{t("agents:nativeUsage.organization")}</dt>
        <dd>{group.organization || unknown}</dd>
        <dt>{t("agents:nativeUsage.observedAt")}</dt>
        <dd>{date(sample?.fetched_at)}</dd>
      </dl>
      {!sample && (
        <p className="text-sm text-muted-foreground">
          {t("sidebar:controlInspection.accountUnknown")}
        </p>
      )}
      <div className="space-y-2 text-sm">
        {sample?.windows.map((window, index) => (
          <div
            key={`${window.label}:${window.model ?? ""}:${index}`}
            className="flex flex-wrap gap-x-3 gap-y-1"
            data-testid="control-provider-window"
          >
            <span>{window.label}</span>
            {window.model && <span>{window.model}</span>}
            <span className="tabular-nums">
              {window.utilization_observed && Number.isFinite(window.utilization_pct)
                ? `${window.utilization_pct}%`
                : unknown}
            </span>
            <span>
              {t("agents:nativeUsage.reset")}:{" "}
              {window.reset_at_observed ? date(window.reset_at) : unknown}
            </span>
          </div>
        ))}
        {sample && sample.windows.length === 0 && (
          <p>{t("sidebar:controlRestore.windowsUnknown")}</p>
        )}
      </div>
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {latestModelUsage(group.entries).map(({ execution_id, report }) => {
          const limit = observedLimitState(report);
          return (
            <div
              key={execution_id}
              className="space-y-1 rounded-md border p-3 text-sm"
              data-testid="control-provider-model"
            >
              <h3 className="break-words font-medium">{report.model || unknown}</h3>
              <p className={limit === "limited" ? "text-destructive" : "text-muted-foreground"}>
                {t(`sidebar:controlRestore.limit.${limit}`)}
              </p>
              {report.binding_changed && (
                <p role="alert">{t("agents:nativeUsage.bindingChanged")}</p>
              )}
              {(!report.source_known || report.limited) && (
                <p className="break-words text-xs">
                  {report.reason ||
                    report.blocking_window ||
                    t("sidebar:controlInspection.usageUnknown")}
                </p>
              )}
              {report.limited && (
                <p className="text-xs">
                  {t("agents:nativeUsage.reset")}: {date(report.reset_at)}
                </p>
              )}
            </div>
          );
        })}
      </div>
    </Card>
  );
}
