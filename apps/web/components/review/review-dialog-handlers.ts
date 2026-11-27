import { hashDiff, isReviewFileDetailReady, reviewFileKey, type ReviewFile } from "./types";

type MarkReviewed = (path: string, hash: string) => void;
type MarkUnreviewed = (path: string) => void;

export function createReviewToggleHandler(
  allFiles: ReviewFile[],
  markReviewed: MarkReviewed,
  markUnreviewed: MarkUnreviewed,
) {
  return (key: string, reviewed: boolean) => {
    if (!reviewed) {
      markUnreviewed(key);
      return;
    }

    const file = allFiles.find((candidate) => reviewFileKey(candidate) === key);
    if (!file || !isReviewFileDetailReady(file)) return;
    markReviewed(key, hashDiff(file.diff));
  };
}
