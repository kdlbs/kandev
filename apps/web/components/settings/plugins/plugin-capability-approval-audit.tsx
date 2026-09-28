import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import type { PluginCapabilityApprovalEvent } from "@/lib/types/plugins";

export function PluginCapabilityApprovalAudit({
  events,
}: {
  events: PluginCapabilityApprovalEvent[];
}) {
  const { t } = useTranslation();
  return (
    <details className="rounded-md border border-border/70 p-3" data-testid="plugin-approval-audit">
      <summary className="cursor-pointer text-sm font-medium">
        {t("plugins:capabilityApprovalAudit")}
      </summary>
      {events.length === 0 ? (
        <p className="mt-3 text-sm text-muted-foreground">
          {t("plugins:noCapabilityApprovalAudit")}
        </p>
      ) : (
        <ol className="mt-3 space-y-3">
          {events.map((event) => (
            <li key={event.audit_id} className="border-l-2 border-border pl-3 text-sm">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="font-medium">{t(`plugins:approvalEvent_${event.type}`)}</span>
                <span className="text-xs text-muted-foreground">
                  {formatAuditTime(event.observed_at)}
                </span>
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                {t("plugins:approvalAuditActorReason", {
                  actor: event.actor,
                  reason: event.reason,
                })}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                {t("plugins:approvalAuditRevision", {
                  before: event.before_revision,
                  after: event.after_revision,
                })}
              </div>
            </li>
          ))}
        </ol>
      )}
    </details>
  );
}

function formatAuditTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : formatDateTime(date);
}
