import { useMemo } from "react";
import { parsePatchFiles, parseDiffFromFile } from "@pierre/diffs";
import type { FileDiffMetadata } from "@pierre/diffs";
import type { FileDiffData } from "@/lib/diff/types";

/**
 * Check if Go code contains patterns that trigger catastrophic regex backtracking
 * in shiki's JavaScript regex engine.
 */
function hasProblematicGoPattern(content: string | undefined): boolean {
  if (!content) return false;
  const problematicPattern = /interface\{\}\s*`[^`]*`/;
  return problematicPattern.test(content);
}

function isGoFile(filePath: string): boolean {
  return filePath.endsWith(".go");
}

/** Parse diff data into FileDiffMetadata, with Go-specific safety checks. */
export function useDiffMetadata(data: FileDiffData): FileDiffMetadata | null {
  return useMemo<FileDiffMetadata | null>(() => {
    let result: FileDiffMetadata | null = null;
    if (data.diff) {
      const parsed = parsePatchFiles(data.diff);
      result = parsed[0]?.files[0] ?? null;
    } else if (data.oldContent || data.newContent) {
      result = parseDiffFromFile(
        { name: data.filePath, contents: data.oldContent },
        { name: data.filePath, contents: data.newContent },
      );
    }

    if (result && isGoFile(data.filePath)) {
      const contentToCheck = data.newContent || data.oldContent || data.diff;
      if (hasProblematicGoPattern(contentToCheck)) {
        result = { ...result, lang: "text" };
      }
    }

    return result;
  }, [data.diff, data.oldContent, data.newContent, data.filePath]);
}
