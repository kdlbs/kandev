import { IconCheck, IconX, IconClock } from "@tabler/icons-react";
import type { CheckRun } from "@/lib/types/github";
import { CollapsibleSection, AddToContextButton, formatDuration, formatElapsed } from "./pr-shared";

function CheckStatusIcon({ check }: { check: CheckRun }) {
  const value = check.conclusion || check.status;
  if (value === "success") return <IconCheck className="h-3.5 w-3.5 text-green-500 shrink-0" />;
  if (value === "failure" || value === "timed_out")
    return <IconX className="h-3.5 w-3.5 text-red-500 shrink-0" />;
  if (value === "in_progress" || value === "queued")
    return <IconClock className="h-3.5 w-3.5 text-yellow-500 shrink-0 animate-pulse" />;
  return <IconClock className="h-3.5 w-3.5 text-muted-foreground shrink-0" />;
}

function conclusionLabel(check: CheckRun): string | null {
  const c = check.conclusion;
  if (!c || c === "success" || c === "failure") return c || null;
  const labels: Record<string, string> = {
    timed_out: "timed out",
    cancelled: "cancelled",
    action_required: "action required",
    skipped: "skipped",
    neutral: "neutral",
    stale: "stale",
  };
  return labels[c] ?? c;
}

function checkDurationText(check: CheckRun): string | null {
  if (check.started_at && check.completed_at)
    return formatDuration(check.started_at, check.completed_at);
  if (check.started_at && !check.completed_at) return `${formatElapsed(check.started_at)} running`;
  return null;
}

function isFailedCheck(check: CheckRun): boolean {
  return check.conclusion === "failure" || check.conclusion === "timed_out";
}

function buildCheckMessage(check: CheckRun): string {
  const parts = [`CI check **${check.name}** failed (${check.conclusion}).`];
  if (check.output) parts.push(check.output);
  if (check.html_url) parts.push(`Check URL: ${check.html_url}`);
  parts.push("Please investigate and fix this failing check.");
  return parts.join("\n\n");
}

function buildAllFailedMessage(checks: CheckRun[]): string {
  const failed = checks.filter(isFailedCheck);
  const parts = [`### ${failed.length} CI Check${failed.length !== 1 ? "s" : ""} Failed`, ""];
  for (const check of failed) {
    parts.push(`**${check.name}** — ${check.conclusion}`);
    if (check.output) parts.push(check.output);
    if (check.html_url) parts.push(`URL: ${check.html_url}`);
    parts.push("");
  }
  parts.push("Please investigate and fix these failing checks.");
  return parts.join("\n");
}

function formatSectionSummary(checks: CheckRun[]): string {
  const failed = checks.filter(isFailedCheck).length;
  const passed = checks.filter((c) => c.conclusion === "success").length;
  const pending = checks.length - failed - passed;
  const parts: string[] = [];
  if (failed > 0) parts.push(`${failed} failed`);
  if (passed > 0) parts.push(`${passed} passed`);
  if (pending > 0) parts.push(`${pending} pending`);
  return parts.join(", ");
}

export function ChecksSection({
  checks,
  onAddAsContext,
}: {
  checks: CheckRun[];
  onAddAsContext: (message: string) => void;
}) {
  const summary = checks.length > 0 ? ` \u2014 ${formatSectionSummary(checks)}` : "";
  const hasFailed = checks.some(isFailedCheck);

  return (
    <CollapsibleSection
      title={`CI Checks${summary}`}
      count={checks.length}
      defaultOpen
      onAddAll={hasFailed ? () => onAddAsContext(buildAllFailedMessage(checks)) : undefined}
      addAllLabel="Add all failed checks to chat context"
    >
      {checks.length === 0 && <p className="text-xs text-muted-foreground px-2 py-2">No checks</p>}
      {checks.map((check) => {
        const label = conclusionLabel(check);
        const duration = checkDurationText(check);
        return (
          <div key={check.name} className="px-2.5 py-2 rounded-md border border-border bg-muted/30">
            <div className="flex items-center gap-2 text-xs">
              <CheckStatusIcon check={check} />
              {check.html_url ? (
                <a
                  href={check.html_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-medium truncate hover:underline cursor-pointer"
                >
                  {check.name}
                </a>
              ) : (
                <span className="font-medium truncate">{check.name}</span>
              )}
              {isFailedCheck(check) && (
                <div className="ml-auto shrink-0">
                  <AddToContextButton onClick={() => onAddAsContext(buildCheckMessage(check))} />
                </div>
              )}
            </div>
            {(label || duration) && (
              <div className="flex items-center gap-1 pl-5.5 mt-0.5 text-[10px] text-muted-foreground">
                {label && <span>{label}</span>}
                {label && duration && <span>&middot;</span>}
                {duration && <span>{duration}</span>}
              </div>
            )}
          </div>
        );
      })}
    </CollapsibleSection>
  );
}
