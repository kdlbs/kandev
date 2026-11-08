import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useControlStalls } from "@/hooks/domains/coordinator/use-control-stalls";
import { useIntegratedControl } from "./integrated-control-provider";
import { useControlInbox } from "./control-inbox-provider";

function BlockerChip({
  label,
  value,
  testId,
}: {
  label: string;
  value: number | null;
  testId: string;
}) {
  const { t } = useTranslation();
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5"
      data-testid={testId}
    >
      {label}
      <b className="font-medium tabular-nums">{value ?? t("common:unknown")}</b>
    </span>
  );
}

function recoveryCount(inbox: ReturnType<typeof useControlInbox>): number | null {
  if (!inbox?.enabled || inbox.failed) return null;
  return Object.values(inbox.byWorkspace).reduce((sum, row) => sum + row.recovery, 0);
}

function stallCount(stalls: ReturnType<typeof useControlStalls>, workspaces: number) {
  const rows = stalls.data ?? [];
  if (stalls.status !== "ready" || rows.length !== workspaces) return null;
  if (!rows.every((row) => row.status === "ready")) return null;
  return rows.reduce((sum, row) => sum + (row.count ?? 0), 0);
}

/**
 * What the recorded state says is blocking work: recovery decisions still
 * waiting in the Needs-you inbox, and tasks the coordinator watchdog reported
 * as stalled. Counts only; the reason values are not stored.
 */
export function ControlBlockerChips() {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const inbox = useControlInbox();
  const coordinatorEnabled = Boolean(control?.coordinatorEnabled);
  const ids = useMemo(
    () =>
      coordinatorEnabled ? (control?.data?.workspaces.map((workspace) => workspace.id) ?? []) : [],
    [control?.data, coordinatorEnabled],
  );
  const stalls = useControlStalls(ids, coordinatorEnabled ? control?.data : undefined);
  if (!control?.data) return null;
  if (!inbox?.enabled && !coordinatorEnabled) return null;
  return (
    <section
      className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
      data-testid="control-blocker-chips"
    >
      <span>{t("sidebar:controlSummary.blockers.title")}</span>
      {inbox?.enabled && (
        <BlockerChip
          label={t("sidebar:controlSummary.blockers.recovery")}
          value={recoveryCount(inbox)}
          testId="control-blocker-recovery"
        />
      )}
      {coordinatorEnabled && (
        <BlockerChip
          label={t("sidebar:controlSummary.blockers.stalls")}
          value={stallCount(stalls, ids.length)}
          testId="control-blocker-stalls"
        />
      )}
    </section>
  );
}
