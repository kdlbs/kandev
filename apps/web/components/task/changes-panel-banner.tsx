import { useMemo } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { reviewFileKey } from "@/components/review/types";

/** Composite key for the file currently selected in single-file mode. */
export function useSelectedFileKey(
  mode: string,
  filePath: string | undefined,
  fileRepositoryName: string | undefined,
): string | undefined {
  return useMemo(
    () =>
      mode === "file" && filePath
        ? reviewFileKey({ path: filePath, repository_name: fileRepositoryName })
        : undefined,
    [mode, filePath, fileRepositoryName],
  );
}

/** Banner shown when the backend dropped files from a huge cumulative diff
 *  (large rebase) to keep the rendered row count bounded. */
export function TruncatedFilesBanner({ count }: { count: number }) {
  if (count <= 0) return null;
  return (
    <div
      data-testid="changes-truncated-banner"
      className="flex items-center gap-2 px-4 py-2 text-xs text-yellow-600 bg-yellow-500/10 border-b border-yellow-500/20"
    >
      <IconAlertTriangle className="h-3.5 w-3.5 shrink-0" />
      <span>
        {count.toLocaleString()} more changed {count === 1 ? "file is" : "files are"} hidden — the
        change set is too large to render in full.
      </span>
    </div>
  );
}
