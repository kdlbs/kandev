"use client";

import { useTranslation } from "react-i18next";

import {
  useWorkspaceRuntimeFootprint,
  type WorkspaceRuntimeFootprintLoader,
} from "@/hooks/domains/settings/use-workspace-runtime-footprint";
import type { WorkspaceRuntimeFootprint } from "@/lib/api/domains/runtime-footprint-api";
import { formatBytes } from "@/lib/utils/format-bytes";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";

type WorkspaceRuntimeFootprintCardProps = {
  workspaceId: string;
  /** Passed through by tests; production reads the live backend. */
  load?: WorkspaceRuntimeFootprintLoader;
};

/**
 * Shows what this workspace's live agent runtimes currently cost.
 *
 * Loading requests one bounded observation. A failed observation is presented as
 * "not measured" rather than as zero memory in use, and a partial measurement is
 * labelled as a lower bound. Both distinctions exist because the alternative
 * reads as "this installation holds no agent memory" during exactly the incident
 * an operator opens this panel to understand.
 */
export function WorkspaceRuntimeFootprintCard({
  workspaceId,
  load,
}: WorkspaceRuntimeFootprintCardProps) {
  const { t } = useTranslation();
  const { footprint, failed } = useWorkspaceRuntimeFootprint(workspaceId, load);

  return (
    <SettingsCard>
      <SettingsCardHeader
        title={t("settings:workspaceRuntimeFootprintTitle")}
        description={t("settings:workspaceRuntimeFootprintDescription")}
      />

      {failed ? (
        <p data-testid="runtime-footprint-error">
          {t("settings:workspaceRuntimeFootprintUnavailable")}
        </p>
      ) : (
        <FootprintBody footprint={footprint} />
      )}
    </SettingsCard>
  );
}

/**
 * Renders one observation.
 *
 * The four non-measured states are kept as separate branches rather than
 * collapsed, because each one is a different thing an operator must not
 * misread. "Not measured" and "no runtimes" in particular both render zero
 * totals if merged, and an operator reading zero during a memory incident would
 * conclude the opposite of what is true.
 */
function FootprintBody({ footprint }: { footprint: WorkspaceRuntimeFootprint | null }) {
  const { t } = useTranslation();

  if (footprint === null) {
    return (
      <p data-testid="runtime-footprint-loading">
        {t("settings:workspaceRuntimeFootprintLoading")}
      </p>
    );
  }
  if (!footprint.complete) {
    return (
      <p data-testid="runtime-footprint-unmeasured">
        {t("settings:workspaceRuntimeFootprintUnmeasured")}
      </p>
    );
  }
  if (footprint.workspaceRuntimes === 0) {
    return (
      <p data-testid="runtime-footprint-empty">{t("settings:workspaceRuntimeFootprintEmpty")}</p>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <FootprintStat
          testId="runtime-footprint-runtimes"
          label={t("settings:workspaceRuntimeFootprintRuntimes")}
          value={String(footprint.workspaceRuntimes)}
        />
        <FootprintStat
          testId="runtime-footprint-processes"
          label={t("settings:workspaceRuntimeFootprintProcesses")}
          value={String(footprint.processes)}
        />
        <FootprintStat
          testId="runtime-footprint-committed"
          label={t("settings:workspaceRuntimeFootprintCommitted")}
          value={formatCommitted(footprint.committedBytes, t)}
        />
        <FootprintStat
          testId="runtime-footprint-resident"
          label={t("settings:workspaceRuntimeFootprintResident")}
          value={formatBytes(footprint.residentBytes)}
        />
      </dl>

      {footprint.unreadableRuntimes > 0 ? (
        <p data-testid="runtime-footprint-partial" className="text-muted-foreground text-sm">
          {t("settings:workspaceRuntimeFootprintPartial")}
        </p>
      ) : null}

      <ul className="flex flex-col gap-1" data-testid="runtime-footprint-rows">
        {footprint.runtimes.map((row) => (
          <li
            key={row.sessionId}
            className="flex flex-wrap items-baseline justify-between gap-2 text-sm"
          >
            <span className="font-mono">{row.sessionId}</span>
            <span className="text-muted-foreground">
              {t("settings:workspaceRuntimeFootprintRowSummary", {
                processes: row.processes,
                resident: formatBytes(row.residentBytes),
              })}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Committed bytes are reported only where the platform measured them. On Linux
 * and other unix hosts the OS does not expose commit charge per process, so this
 * says so instead of deriving a figure from resident bytes, which would present
 * an estimate as a measurement.
 */
function formatCommitted(committedBytes: number, t: (key: string) => string): string {
  if (committedBytes === 0) {
    return t("settings:workspaceRuntimeFootprintUnavailableMetric");
  }
  return formatBytes(committedBytes);
}

function FootprintStat({ testId, label, value }: { testId: string; label: string; value: string }) {
  return (
    <div className="flex flex-col">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="font-medium" data-testid={testId}>
        {value}
      </dd>
    </div>
  );
}
