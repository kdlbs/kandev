import type { TaskReviewFinding } from "@/lib/types/review";

/** Human label for a severity, used in headings and chips. */
export const SEVERITY_LABELS: Record<string, string> = {
  blocker: "Blocker",
  major: "Major",
  minor: "Minor",
  nit: "Nit",
};

export function severityLabel(severity: string): string {
  return SEVERITY_LABELS[severity] ?? severity;
}

/** `repo/path:12-14` or `path:12`, the form used in headings and agent context. */
export function findingLocation(finding: TaskReviewFinding): string {
  const path = finding.repository_name
    ? `${finding.repository_name}/${finding.file_path}`
    : finding.file_path;
  if (finding.end_line > finding.start_line) {
    return `${path}:${finding.start_line}-${finding.end_line}`;
  }
  return `${path}:${finding.start_line}`;
}

/**
 * Renders a finding as the markdown block sent to an agent as follow-up context.
 *
 * Mirrors the shape of `formatReviewCommentsAsMarkdown` so a finding reads to the
 * agent like any other anchored review feedback, and states plainly that the
 * suggestion was not applied — the agent must not assume the fix is already in
 * the working tree.
 */
export function formatFindingAsMarkdown(finding: TaskReviewFinding): string {
  const lines: string[] = ["### Code Review Finding", ""];
  lines.push(`**${findingLocation(finding)}** — ${severityLabel(finding.severity)}`);
  if (finding.category) lines.push(`Category: ${finding.category}`);
  lines.push("", `**${finding.title}**`, "", finding.body);
  if (finding.suggestion) {
    lines.push("", "Suggested change (not applied):", "```", finding.suggestion, "```");
  }
  lines.push("", "---", "");
  return lines.join("\n");
}

/** Renders several findings as one markdown block. */
export function formatFindingsAsMarkdown(findings: TaskReviewFinding[]): string {
  return findings.map(formatFindingAsMarkdown).join("\n");
}
