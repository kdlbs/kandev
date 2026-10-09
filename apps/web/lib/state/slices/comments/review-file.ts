import { generateUUID } from "@/lib/utils";
import type { ReviewFileComment } from "./types";

export type ReviewFileCommentTarget = {
  sessionId: string;
  filePath: string;
  repositoryName: string;
  repositoryId?: string;
  baseRef?: string;
  isSubmodule?: boolean;
};

export function buildReviewFileComment(
  target: ReviewFileCommentTarget,
  text: string,
): ReviewFileComment | null {
  const trimmed = text.trim();
  if (!trimmed || !target.sessionId) return null;
  return {
    ...target,
    id: generateUUID(),
    source: "review-file",
    text: trimmed,
    status: "pending",
    createdAt: new Date().toISOString(),
  };
}
