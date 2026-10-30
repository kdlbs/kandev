import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { formatNumber } from "@/lib/i18n/formats";
import { controlHref } from "@/lib/navigation/integrated-control";
import type { NativeDashboard } from "@/lib/types/control-native";
import {
  waitReasonCounts,
  WAIT_REASONS,
  type WaitReason,
  type WaitReasonCounts,
} from "./control-wait-reasons";

export type WaitReasonWorkspace = { id: string; name: string };
export type WaitReasonRow = {
  workspace: WaitReasonWorkspace;
  data?: NativeDashboard;
};

const REASON_LABELS: Record<WaitReason, string> = {
  input: "sidebar:controlSummary.waitReasons.input",
  blocked: "sidebar:controlSummary.waitReasons.blocked",
  approval: "sidebar:controlSummary.waitReasons.approval",
  hold: "sidebar:controlSummary.waitReasons.hold",
};

function ReasonValue({ reason, counts }: { reason: WaitReason; counts: WaitReasonCounts }) {
  const { t } = useTranslation();
  const value = counts[reason];
  return (
    <dd
      className="font-mono tabular-nums"
      data-testid="control-wait-reason-value"
      data-reason={reason}
    >
      {value === null ? t("common:unknown") : formatNumber(value)}
    </dd>
  );
}

/**
 * Why recorded work is not executing, per authorized workspace. Every value is
 * a task count from the same Native dashboard read the comparison uses; a
 * loading, denied or failed read is unknown, not a measured zero.
 */
export function ControlWaitReasons({ rows }: { rows: WaitReasonRow[] }) {
  const { t } = useTranslation();
  if (rows.length === 0) return null;
  return (
    <section className="space-y-2" data-testid="control-wait-reasons">
      <h3 className="text-sm font-medium">{t("sidebar:controlSummary.waitReasons.title")}</h3>
      <div className="divide-y rounded-lg border">
        {rows.map(({ workspace, data }) => {
          const counts = waitReasonCounts(data);
          return (
            <div
              key={workspace.id}
              className="p-3"
              data-testid="control-wait-reason-row"
              data-workspace-id={workspace.id}
            >
              <Link
                href={controlHref("workspaces", workspace.id)}
                className="inline-flex min-h-7 cursor-pointer items-center font-medium underline-offset-4 hover:underline max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              >
                {workspace.name}
              </Link>
              {counts ? (
                <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-sm sm:grid-cols-4">
                  {WAIT_REASONS.map((reason) => (
                    <div
                      key={reason}
                      className="flex items-baseline justify-between gap-2 sm:block"
                    >
                      <dt className="text-muted-foreground">{t(REASON_LABELS[reason])}</dt>
                      <ReasonValue reason={reason} counts={counts} />
                    </div>
                  ))}
                </dl>
              ) : (
                <p className="mt-2 text-sm text-muted-foreground">{t("common:unknown")}</p>
              )}
            </div>
          );
        })}
      </div>
      <p className="text-xs text-muted-foreground">
        {t("sidebar:controlSummary.waitReasons.basis")}
      </p>
    </section>
  );
}
