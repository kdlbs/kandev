import { reviewFileKey } from "@/components/review/types";
import type { ReviewSeverity, TaskReviewFinding, TaskReviewRun } from "@/lib/types/review";

/**
 * Composite per-file key for a finding, using the same `<repo>\x00<path>` shape
 * as `reviewFileKey` so findings, review marks, comment counts, and the file
 * tree all agree on one identity. Without this, two same-named files in
 * different repositories would share their findings.
 */
export function findingFileKey(finding: TaskReviewFinding): string {
  return reviewFileKey({
    path: finding.file_path,
    repository_name: finding.repository_name || undefined,
  });
}

/**
 * True when the file's diff has changed since the finding was written.
 *
 * Reuses the per-file diff-hash mechanism that already drives review-mark
 * staleness. A finding with no stored hash (published by an agent through MCP
 * rather than by a review pass over a collected change set) is never reported
 * stale, because there is no reviewed diff to compare against.
 */
export function isFindingStale(finding: TaskReviewFinding, currentDiffHash: string | undefined) {
  if (!finding.file_diff_hash || !currentDiffHash) return false;
  return finding.file_diff_hash !== currentDiffHash;
}

/** A finding's line range, possibly relocated from its stored anchor. */
export type FindingAnchor = {
  startLine: number;
  endLine: number;
  /** True when the range was recovered from `anchor_text` after the diff moved. */
  relocated: boolean;
};

/**
 * Best-effort relocation of a stale finding within the same file.
 *
 * The stored `anchor_text` is the new-side content the reviewer actually looked
 * at. If that text still exists in the current diff, the finding follows the
 * code it was written about; if it does not, we return null so the caller can
 * surface it as stale rather than anchor it to whatever now occupies the stored
 * line number.
 */
export function reanchorFinding(
  finding: TaskReviewFinding,
  diff: string | undefined,
): FindingAnchor | null {
  const anchorText = finding.anchor_text?.trim();
  if (!anchorText || !diff) return null;

  const anchorLines = anchorText.split("\n").map((line) => line.trim());
  const newSide = newSideLines(diff);
  if (newSide.length === 0) return null;

  for (let start = 0; start + anchorLines.length <= newSide.length; start++) {
    let matched = true;
    for (let offset = 0; offset < anchorLines.length; offset++) {
      if (newSide[start + offset].text.trim() !== anchorLines[offset]) {
        matched = false;
        break;
      }
    }
    if (!matched) continue;
    const startLine = newSide[start].lineNumber;
    const endLine = newSide[start + anchorLines.length - 1].lineNumber;
    return {
      startLine,
      endLine,
      relocated: startLine !== finding.start_line || endLine !== finding.end_line,
    };
  }
  return null;
}

type NewSideLine = { lineNumber: number; text: string };

const HUNK_HEADER = /^@@+ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

/**
 * Splits a diff into lines, dropping the empty element a trailing newline leaves
 * behind. That artifact carries no diff marker, so counting it as a context line
 * would add a phantom line past the end of the hunk and let an anchor match
 * against it.
 */
function diffBodyLines(diff: string): string[] {
  const lines = diff.split("\n");
  if (lines.length > 0 && lines[lines.length - 1] === "") return lines.slice(0, -1);
  return lines;
}

/**
 * Extracts the new-side lines of a unified diff with their line numbers.
 * Deletions are skipped because they exist only on the old side and must not
 * advance the new-side counter.
 */
function newSideLines(diff: string): NewSideLine[] {
  const result: NewSideLine[] = [];
  let lineNumber = 0;
  let inHunk = false;

  for (const raw of diffBodyLines(diff)) {
    const line = raw.replace(/\r$/, "");
    const header = line.match(HUNK_HEADER);
    if (header) {
      lineNumber = Number(header[1]);
      inHunk = true;
      continue;
    }
    if (!inHunk) continue;
    if (line.startsWith("-") || line.startsWith("\\")) continue;
    if (line === "" || line.startsWith("+") || line.startsWith(" ")) {
      result.push({ lineNumber, text: line === "" ? "" : line.slice(1) });
      lineNumber++;
      continue;
    }
    // "diff --git", "index", "+++", "---": file metadata between hunks.
    inHunk = false;
  }
  return result;
}

/**
 * Resolves where a finding should render.
 *
 * Returns null when the finding cannot be placed inline — either its file's diff
 * moved and the anchor text is gone, or it has no anchor text at all. Callers
 * render those in the per-file stale group instead of guessing a line.
 */
export function resolveFindingAnchor(
  finding: TaskReviewFinding,
  diff: string | undefined,
  currentDiffHash: string | undefined,
): FindingAnchor | null {
  if (!isFindingStale(finding, currentDiffHash)) {
    return { startLine: finding.start_line, endLine: finding.end_line, relocated: false };
  }
  return reanchorFinding(finding, diff);
}

const SEVERITY_ORDER: Record<ReviewSeverity, number> = {
  blocker: 0,
  major: 1,
  minor: 2,
  nit: 3,
};

/** Rank for severity ordering; unknown severities sort last. */
export function severityRank(severity: ReviewSeverity | string): number {
  return SEVERITY_ORDER[severity as ReviewSeverity] ?? Number.MAX_SAFE_INTEGER;
}

/** Sorts by severity, then repository, then file, then line — stable for render. */
export function sortFindings(findings: TaskReviewFinding[]): TaskReviewFinding[] {
  return [...findings].sort((a, b) => {
    const bySeverity = severityRank(a.severity) - severityRank(b.severity);
    if (bySeverity !== 0) return bySeverity;
    const byRepo = (a.repository_name || "").localeCompare(b.repository_name || "");
    if (byRepo !== 0) return byRepo;
    const byFile = a.file_path.localeCompare(b.file_path);
    if (byFile !== 0) return byFile;
    return a.start_line - b.start_line;
  });
}

/** Groups findings by their composite file key. */
export function groupFindingsByFile(
  findings: TaskReviewFinding[],
): Map<string, TaskReviewFinding[]> {
  const byFile = new Map<string, TaskReviewFinding[]>();
  for (const finding of findings) {
    const key = findingFileKey(finding);
    const existing = byFile.get(key);
    if (existing) existing.push(finding);
    else byFile.set(key, [finding]);
  }
  return byFile;
}

export type RepositoryFindingGroup = {
  repositoryName: string;
  findings: TaskReviewFinding[];
};

/**
 * Groups findings per repository for the overview, matching how the rest of the
 * Changes panel groups a multi-repository task. A single-repository task yields
 * one group with an empty name.
 */
export function groupFindingsByRepository(findings: TaskReviewFinding[]): RepositoryFindingGroup[] {
  const byRepo = new Map<string, TaskReviewFinding[]>();
  for (const finding of sortFindings(findings)) {
    const name = finding.repository_name || "";
    const existing = byRepo.get(name);
    if (existing) existing.push(finding);
    else byRepo.set(name, [finding]);
  }
  return Array.from(byRepo.entries())
    .map(([repositoryName, group]) => ({ repositoryName, findings: group }))
    .sort((a, b) => a.repositoryName.localeCompare(b.repositoryName));
}

/** Number of findings the human has not yet resolved or dismissed. */
export function openFindingCount(findings: TaskReviewFinding[]): number {
  return findings.filter((f) => f.status === "open").length;
}

/** Per-file open-finding counts, keyed like `reviewFileKey`. */
export function openFindingCountByFile(findings: TaskReviewFinding[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const finding of findings) {
    if (finding.status !== "open") continue;
    const key = findingFileKey(finding);
    counts[key] = (counts[key] ?? 0) + 1;
  }
  return counts;
}

/** True while a run is still in flight and can be cancelled. */
export function isRunActive(run: TaskReviewRun | null | undefined): boolean {
  return run?.status === "pending" || run?.status === "running";
}

/** The task's newest run, which is the one the toolbar reflects. */
export function latestRun(runs: TaskReviewRun[]): TaskReviewRun | null {
  if (runs.length === 0) return null;
  return [...runs].sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
}
