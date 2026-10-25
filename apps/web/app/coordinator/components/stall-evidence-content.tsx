import { useTranslation } from "react-i18next";
import { PopoverContent, PopoverHeader, PopoverTitle } from "@kandev/ui/popover";
import {
  isRunningSessionState,
  type AttentionStall,
  type AttentionTask,
} from "@/lib/coordinator/attention";
import { formatAge, formatEvidenceTime } from "@/lib/coordinator/format";

export type StallEvidenceContentProps = {
  task: AttentionTask;
  stall: AttentionStall;
};

/**
 * Stalled for, last event at, detected at, and whether an agent is running
 * now, from the task's primary session state (AC-COORDINATOR-NEEDS-YOU-002.6).
 */
export function StallEvidenceContent({ task, stall }: StallEvidenceContentProps) {
  const { t } = useTranslation();
  const running = isRunningSessionState(task.statusSummary?.primary_session?.state ?? undefined);

  return (
    <PopoverContent>
      <PopoverHeader>
        <PopoverTitle>{t("coordinator:showEvidence")}</PopoverTitle>
      </PopoverHeader>
      <dl className="grid grid-cols-2 gap-x-2 gap-y-1 text-xs">
        <dt className="text-muted-foreground">{t("coordinator:evidenceStalledFor")}</dt>
        <dd>{formatAge(stall.stalled_for_ms)}</dd>
        <dt className="text-muted-foreground">{t("coordinator:evidenceLastEventAt")}</dt>
        <dd>{formatEvidenceTime(stall.last_event_at)}</dd>
        <dt className="text-muted-foreground">{t("coordinator:evidenceDetectedAt")}</dt>
        <dd>{formatEvidenceTime(stall.detected_at)}</dd>
        <dt className="text-muted-foreground">{t("coordinator:evidenceAgentStatus")}</dt>
        <dd>
          {running
            ? t("coordinator:evidenceAgentRunning")
            : t("coordinator:evidenceAgentNotRunning")}
        </dd>
      </dl>
    </PopoverContent>
  );
}
