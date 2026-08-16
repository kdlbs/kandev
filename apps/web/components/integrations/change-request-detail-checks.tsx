import { useEffect, useState } from "react";
import { IconCheck, IconClock, IconX } from "@tabler/icons-react";
import {
  AddToContextButton,
  CollapsibleSection,
  formatDuration,
  formatElapsed,
} from "@/components/github/pr-shared";
import { t } from "@/lib/i18n";
import type {
  ChangeRequestDetailCheck,
  ChangeRequestDetailModel,
  ChangeRequestDetailProps,
} from "./change-request-detail";

function CheckIcon({ check }: { check: ChangeRequestDetailCheck }) {
  const state = (check.conclusion || check.state).toLowerCase();
  if (["success", "successful", "passed"].includes(state)) {
    return <IconCheck className="h-3.5 w-3.5 shrink-0 text-green-500" />;
  }
  if (["failure", "failed", "error", "timed_out"].includes(state)) {
    return <IconX className="h-3.5 w-3.5 shrink-0 text-red-500" />;
  }
  if (["in_progress", "running", "queued", "pending"].includes(state)) {
    return <IconClock className="h-3.5 w-3.5 shrink-0 animate-pulse text-yellow-500" />;
  }
  return <IconClock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />;
}

function isFailedCheck(check: ChangeRequestDetailCheck) {
  return ["failure", "failed", "error", "timed_out"].includes(
    (check.conclusion || check.state).toLowerCase(),
  );
}

function checkConclusionLabel(check: ChangeRequestDetailCheck) {
  const value = (check.conclusion || check.state).toLowerCase();
  const labels: Record<string, string> = {
    timed_out: t("integrations:timedOut"),
    action_required: t("integrations:actionRequired"),
    in_progress: t("integrations:inProgress").toLowerCase(),
  };
  return labels[value] ?? value.replaceAll("_", " ");
}

function checkSummary(checks: ChangeRequestDetailCheck[]) {
  const failed = checks.filter(isFailedCheck).length;
  const passed = checks.filter((check) =>
    ["success", "successful", "passed"].includes((check.conclusion || check.state).toLowerCase()),
  ).length;
  const pending = checks.length - failed - passed;
  return [
    failed > 0 ? t("integrations:failedCount", { count: failed }) : "",
    passed > 0 ? t("integrations:passedCount", { count: passed }) : "",
    pending > 0 ? t("integrations:pendingCount", { count: pending }) : "",
  ].filter(Boolean);
}

// i18n-exempt: builds the message body sent to the agent, not UI copy.
function buildCheckContext(check: ChangeRequestDetailCheck) {
  return [
    `CI check **${check.name}** failed (${checkConclusionLabel(check)}).`,
    check.output ?? "",
    check.url ? `Check URL: ${check.url}` : "",
    "Please investigate and fix this failing check.",
  ]
    .filter(Boolean)
    .join("\n\n");
}

function buildAllFailedChecksContext(checks: ChangeRequestDetailCheck[]) {
  const failed = checks.filter(isFailedCheck);
  const lines = [`### ${failed.length} CI Check${failed.length === 1 ? "" : "s"} Failed`, ""];
  for (const check of failed) {
    lines.push(`**${check.name}**: ${checkConclusionLabel(check)}`);
    if (check.output) lines.push(check.output);
    if (check.url) lines.push(`URL: ${check.url}`);
    lines.push("");
  }
  lines.push("Please investigate and fix these failing checks.");
  return lines.join("\n");
}

function checkDurationText(check: ChangeRequestDetailCheck) {
  if (!check.startedAt) return null;
  if (!check.completedAt) return `${formatElapsed(check.startedAt)} ${t("integrations:running")}`;
  const milliseconds = new Date(check.completedAt).getTime() - new Date(check.startedAt).getTime();
  return milliseconds < 1_000 ? null : formatDuration(check.startedAt, check.completedAt);
}

export function ChangeRequestChecks({
  detail,
  onAdd,
}: {
  detail: ChangeRequestDetailModel;
  onAdd?: ChangeRequestDetailProps["onAddContext"];
}) {
  const hasRunning = detail.checks.some((check) => check.startedAt && !check.completedAt);
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!hasRunning) return;
    const interval = setInterval(() => setTick((tick) => tick + 1), 1_000);
    return () => clearInterval(interval);
  }, [hasRunning]);
  const summary = checkSummary(detail.checks);
  const summaryLabel = summary.length > 0 ? `: ${summary.join(", ")}` : "";
  const failed = detail.checks.filter(isFailedCheck);
  return (
    <CollapsibleSection
      title={t("integrations:ciChecks", { summary: summaryLabel })}
      count={detail.checks.length}
      defaultOpen
      onAddAll={
        onAdd && failed.length > 0
          ? () => onAdd("check", buildAllFailedChecksContext(detail.checks))
          : undefined
      }
      addAllLabel={t("integrations:addAllFailedChecksToChatContext")}
    >
      {detail.checks.length === 0 ? (
        <p className="px-2 py-2 text-xs text-muted-foreground">{t("integrations:noChecks")}</p>
      ) : null}
      {detail.checks.map((check) => {
        const duration = checkDurationText(check);
        return (
          <div
            key={check.id}
            data-testid={`check-run-${check.name}`}
            className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5 text-xs"
          >
            <CheckIcon check={check} />
            {check.url ? (
              <a
                href={check.url}
                target="_blank"
                rel="noopener noreferrer"
                className="min-w-0 truncate font-medium hover:underline"
              >
                {check.name}
              </a>
            ) : (
              <span className="min-w-0 truncate font-medium">{check.name}</span>
            )}
            <span className="shrink-0 text-[10px] text-muted-foreground">
              {checkConclusionLabel(check)}
            </span>
            {duration ? (
              <span
                data-testid={`check-duration-${check.name}`}
                className="shrink-0 text-[10px] text-muted-foreground"
              >
                {duration}
              </span>
            ) : null}
            {onAdd && isFailedCheck(check) ? (
              <div className="ml-auto [&_button]:h-11 [&_button]:w-11 sm:[&_button]:h-6 sm:[&_button]:w-6">
                <AddToContextButton onClick={() => onAdd("check", buildCheckContext(check))} />
              </div>
            ) : null}
          </div>
        );
      })}
    </CollapsibleSection>
  );
}
