import { createTwoFilesPatch } from "diff";

import { djb2Hash } from "./hash";

/**
 * Generate a unified diff between original and modified content
 * @param original - Original file content
 * @param modified - Modified file content
 * @param filename - File name (used in diff header)
 * @returns Unified diff string
 */
export function generateUnifiedDiff(original: string, modified: string, filename: string): string {
  // createTwoFilesPatch generates a unified diff with headers
  // Parameters: oldFileName, newFileName, oldStr, newStr, oldHeader, newHeader, options
  // Note: oldHeader and newHeader should be empty strings or valid timestamps
  // The go-diff library on the backend expects standard unified diff format
  const diff = createTwoFilesPatch(
    filename,
    filename,
    original,
    modified,
    "", // Empty oldHeader - backend doesn't need timestamps
    "", // Empty newHeader - backend doesn't need timestamps
    { context: 3 }, // Number of context lines
  );

  return diff;
}

/**
 * Calculate hash of content for change detection.
 * Uses djb2 — a fast, non-cryptographic hash sufficient for detecting content changes.
 * @param content - Content to hash
 * @returns Hex-encoded hash
 */
export async function calculateHash(content: string): Promise<string> {
  return djb2Hash(content);
}

/**
 * Calculate diff stats (additions and deletions)
 * @param diff - Unified diff string
 * @returns Object with additions and deletions count
 */
export function calculateDiffStats(diff: string): { additions: number; deletions: number } {
  const lines = diff.split("\n");
  let additions = 0;
  let deletions = 0;

  for (const line of lines) {
    if (line.startsWith("+") && !line.startsWith("+++")) {
      additions++;
    } else if (line.startsWith("-") && !line.startsWith("---")) {
      deletions++;
    }
  }

  return { additions, deletions };
}

/**
 * Format diff stats for display
 * @param additions - Number of additions
 * @param deletions - Number of deletions
 * @returns Formatted string like "+5 -3"
 */
export function formatDiffStats(additions: number, deletions: number): string {
  const parts: string[] = [];
  if (additions > 0) {
    parts.push(`+${additions}`);
  }
  if (deletions > 0) {
    parts.push(`-${deletions}`);
  }
  return parts.join(" ") || "No changes";
}
