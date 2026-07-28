"use client";

import { IconSparkles } from "@tabler/icons-react";
import { ReviewFindingSeverityBadge } from "@/components/diff/review-finding-severity";
import { findingFileKey, sortFindings } from "@/lib/review/findings";
import { formatLineRange } from "@/lib/diff";
import type { TaskReviewFinding } from "@/lib/types/review";

type FindingFileGroup = { key: string; filePath: string; findings: TaskReviewFinding[] };

/**
 * Groups open findings by file, most-severe file first. Findings are pre-sorted
 * by severity → repo → file → line, then grouped by their composite file key so
 * two same-named files in different repos stay distinct and the highest-severity
 * finding drives where its file appears in the list.
 */
export function groupOpenFindingsByFile(findings: TaskReviewFinding[]): FindingFileGroup[] {
  const order: string[] = [];
  const byKey = new Map<string, TaskReviewFinding[]>();
  const pathByKey = new Map<string, string>();
  for (const finding of sortFindings(findings.filter((f) => f.status === "open"))) {
    const key = findingFileKey(finding);
    const existing = byKey.get(key);
    if (existing) {
      existing.push(finding);
    } else {
      order.push(key);
      pathByKey.set(key, displayPath(finding));
      byKey.set(key, [finding]);
    }
  }
  return order.map((key) => ({ key, filePath: pathByKey.get(key)!, findings: byKey.get(key)! }));
}

function displayPath(finding: TaskReviewFinding): string {
  return finding.repository_name
    ? `${finding.repository_name}/${finding.file_path}`
    : finding.file_path;
}

function fileName(filePath: string): string {
  const idx = filePath.lastIndexOf("/");
  return idx === -1 ? filePath : filePath.slice(idx + 1);
}

function fileDir(filePath: string): string {
  const idx = filePath.lastIndexOf("/");
  return idx === -1 ? "" : filePath.slice(0, idx);
}

/**
 * Scrollable list of open findings, grouped per file, each row a jump target.
 * Rendered inside the findings-count popover on the review top bar so the user
 * can go straight to a finding instead of scrolling the diff to hunt for it.
 */
export function ReviewFindingsOverview({
  findings,
  onNavigate,
}: {
  findings: TaskReviewFinding[];
  onNavigate: (finding: TaskReviewFinding) => void;
}) {
  const groups = groupOpenFindingsByFile(findings);
  const total = groups.reduce((sum, g) => sum + g.findings.length, 0);

  if (total === 0) {
    return (
      <div className="px-3 py-4 text-center text-xs text-muted-foreground">No open findings.</div>
    );
  }

  return (
    <div className="flex max-h-[min(60vh,26rem)] flex-col" data-testid="review-findings-overview">
      <div className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <IconSparkles className="h-4 w-4 shrink-0 text-primary" />
        <span className="text-sm font-medium">
          {total} open finding{total !== 1 ? "s" : ""}
        </span>
        <span className="text-xs text-muted-foreground">
          across {groups.length} file{groups.length !== 1 ? "s" : ""}
        </span>
      </div>

      <div
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 py-1.5"
        data-testid="review-findings-overview-scroll"
      >
        {groups.map((group) => {
          const dir = fileDir(group.filePath);
          return (
            <div key={group.key} className="mb-2 last:mb-0">
              <div className="flex items-baseline gap-1.5 px-1 pb-1">
                <span
                  className="truncate text-xs font-medium text-foreground"
                  title={group.filePath}
                >
                  {fileName(group.filePath)}
                </span>
                {dir && (
                  <span className="min-w-0 flex-1 truncate text-[11px] text-muted-foreground/70">
                    {dir}
                  </span>
                )}
                <span className="ml-auto shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                  {group.findings.length}
                </span>
              </div>

              <div className="space-y-1">
                {group.findings.map((finding) => (
                  <button
                    key={finding.id}
                    type="button"
                    onClick={() => onNavigate(finding)}
                    className="w-full cursor-pointer rounded-md border border-border/50 bg-muted/30 px-2 py-1.5 text-left transition-colors hover:border-border hover:bg-muted/60"
                    data-testid="review-finding-nav-item"
                    data-finding-id={finding.id}
                  >
                    <div className="mb-0.5 flex items-center gap-1.5">
                      <ReviewFindingSeverityBadge severity={finding.severity} />
                      <span className="text-[10px] font-medium text-muted-foreground">
                        {formatLineRange(finding.start_line, finding.end_line)}
                      </span>
                    </div>
                    <p className="line-clamp-2 text-xs font-medium leading-snug text-foreground/90">
                      {finding.title}
                    </p>
                  </button>
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
