import { createTwoFilesPatch } from 'diff';

/**
 * Generate a unified diff between original and modified content
 * @param original - Original file content
 * @param modified - Modified file content
 * @param filename - File name (used in diff header)
 * @returns Unified diff string
 */
export function generateUnifiedDiff(
  original: string,
  modified: string,
  filename: string
): string {
  // createTwoFilesPatch generates a unified diff with headers
  // Parameters: oldFileName, newFileName, oldStr, newStr, oldHeader, newHeader, options
  // Note: oldHeader and newHeader should be empty strings or valid timestamps
  // The go-diff library on the backend expects standard unified diff format
  const diff = createTwoFilesPatch(
    filename,
    filename,
    original,
    modified,
    '', // Empty oldHeader - backend doesn't need timestamps
    '', // Empty newHeader - backend doesn't need timestamps
    { context: 3 } // Number of context lines
  );

  return diff;
}

/**
 * Calculate SHA256 hash of content
 * @param content - Content to hash
 * @returns Hex-encoded SHA256 hash
 */
export async function calculateHash(content: string): Promise<string> {
  // Use Web Crypto API for SHA256 hashing
  const encoder = new TextEncoder();
  const data = encoder.encode(content);
  const hashBuffer = await crypto.subtle.digest('SHA-256', data);
  
  // Convert buffer to hex string
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
  
  return hashHex;
}

/**
 * Calculate diff stats (additions and deletions)
 * @param diff - Unified diff string
 * @returns Object with additions and deletions count
 */
export function calculateDiffStats(diff: string): { additions: number; deletions: number } {
  const lines = diff.split('\n');
  let additions = 0;
  let deletions = 0;

  for (const line of lines) {
    if (line.startsWith('+') && !line.startsWith('+++')) {
      additions++;
    } else if (line.startsWith('-') && !line.startsWith('---')) {
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
  return parts.join(' ') || 'No changes';
}

