import { useMemo } from "react";
import type { AnnotationSide, DiffLineAnnotation } from "@pierre/diffs";
import { useOptionalAppStore } from "@/components/state-provider";
import { resolveFindingAnchor } from "@/lib/review/findings";
import { hashDiff } from "@/components/review/types";
import type { TaskReviewFinding } from "@/lib/types/review";
import type { AnnotationMetadata } from "./use-diff-annotation-renderer";

/** Message shown on a finding whose line moved but whose anchor text survived. */
const RELOCATED_REASON =
  "The diff changed after this review; the finding was re-anchored to the code it describes.";

export type ReviewFindingAnnotationsResult = {
  /** Findings that could be anchored to a current line in this file. */
  annotations: DiffLineAnnotation<AnnotationMetadata>[];
  /**
   * Findings for this file that could not be anchored — the diff moved and the
   * anchor text is gone. The caller renders these above the diff so a stale
   * finding is neither dropped nor placed against unrelated code.
   */
  unanchored: TaskReviewFinding[];
};

const EMPTY_RESULT: ReviewFindingAnnotationsResult = { annotations: [], unanchored: [] };

/**
 * Builds inline annotations for the review findings belonging to one file.
 *
 * Findings are matched by the same composite `<repo>\x00<path>` key the rest of
 * the Review panel uses, so in a multi-repository task a finding renders only
 * inside its own repository's file.
 */
export function useReviewFindingAnnotations(opts: {
  filePath: string;
  repo?: string;
  diff?: string;
}): ReviewFindingAnnotationsResult {
  const { filePath, repo, diff } = opts;
  // The diff reaching the viewer is already normalized (see normalizeDiffContent
  // in components/review/types.ts), so hashing it here yields the same value the
  // backend stamped on the finding and the same one computeReviewSets uses for
  // review-mark staleness.
  const diffHash = useMemo(() => (diff ? hashDiff(diff) : undefined), [diff]);
  const activeTaskId = useOptionalAppStore((s) => s.tasks.activeTaskId, null);
  const findings = useOptionalAppStore(
    (s) => (activeTaskId ? s.taskReview.findingsByTaskId[activeTaskId] : undefined),
    undefined,
  );

  return useMemo(() => {
    if (!findings?.length) return EMPTY_RESULT;
    const forFile = findings.filter(
      (f) => f.file_path === filePath && (f.repository_name || undefined) === (repo || undefined),
    );
    if (forFile.length === 0) return EMPTY_RESULT;

    const annotations: DiffLineAnnotation<AnnotationMetadata>[] = [];
    const unanchored: TaskReviewFinding[] = [];

    for (const finding of forFile) {
      const anchor = resolveFindingAnchor(finding, diff, diffHash);
      if (!anchor) {
        unanchored.push(finding);
        continue;
      }
      annotations.push({
        side: (finding.side === "deletions" ? "deletions" : "additions") as AnnotationSide,
        lineNumber: Math.max(anchor.startLine, anchor.endLine),
        metadata: {
          type: "review-finding" as const,
          finding,
          findingStaleReason: anchor.relocated ? RELOCATED_REASON : undefined,
        },
      });
    }
    return { annotations, unanchored };
  }, [findings, filePath, repo, diff, diffHash]);
}
