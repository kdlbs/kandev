import { createTwoFilesPatch } from "diff";

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
 * Simple hash function for non-secure contexts (HTTP).
 * Uses djb2 algorithm - not cryptographically secure but sufficient for
 * content change detection.
 */
function simpleHash(str: string): string {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = (hash * 33) ^ str.charCodeAt(i);
  }
  // Convert to unsigned 32-bit integer, then to hex
  return (hash >>> 0).toString(16).padStart(8, "0");
}

/**
 * Calculate hash of content for change detection.
 * Uses Web Crypto API (SHA-256) when available, falls back to simple hash
 * in non-secure contexts (HTTP).
 * @param content - Content to hash
 * @returns Hex-encoded hash
 */
export async function calculateHash(content: string): Promise<string> {
  // crypto.subtle is only available in secure contexts (HTTPS, localhost)
  if (typeof crypto !== "undefined" && crypto.subtle) {
    try {
      const encoder = new TextEncoder();
      const data = encoder.encode(content);
      const hashBuffer = await crypto.subtle.digest("SHA-256", data);
      const hashArray = Array.from(new Uint8Array(hashBuffer));
      return hashArray.map((b) => b.toString(16).padStart(2, "0")).join("");
    } catch {
      // Fall through to simple hash
    }
  }

  // Fallback for non-secure contexts (HTTP)
  return simpleHash(content);
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
