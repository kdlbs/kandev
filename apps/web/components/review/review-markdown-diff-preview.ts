import type { ReviewFile } from "./types";

export type ReviewMarkdownPreview = {
  fragments: string[];
  isComplete: boolean;
  isPartial: boolean;
};

function extractNewSideHunks(diff: string): string[] {
  const hunks: string[] = [];
  let lines: string[] | null = null;

  for (const line of diff.split("\n")) {
    if (line.startsWith("@@")) {
      if (lines?.length) hunks.push(lines.join("\n"));
      lines = [];
      continue;
    }
    if (!lines || line.startsWith("\\ No newline")) continue;
    if (line.startsWith("+") || line.startsWith(" ")) lines.push(line.slice(1));
  }
  if (lines?.length) hunks.push(lines.join("\n"));
  return hunks;
}

export function extractReviewMarkdownPreview(
  file: Pick<ReviewFile, "diff" | "status" | "diff_skip_reason">,
): ReviewMarkdownPreview {
  const hunks = extractNewSideHunks(file.diff);
  const isComplete =
    (file.status === "added" || file.status === "untracked") &&
    !file.diff_skip_reason &&
    hunks.length === 1;
  return { fragments: hunks, isComplete, isPartial: !isComplete };
}
