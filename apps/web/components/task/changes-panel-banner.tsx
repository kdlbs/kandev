import { useMemo } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { reviewFileKey } from "@/components/review/types";
import type { GitChangeLayer } from "@/lib/state/slices/session-runtime/types";
import { useTranslation } from "react-i18next";

/** Composite key for the file currently selected in single-file mode. */
export function useSelectedFileKey(
  mode: "all" | "file",
  filePath: string | undefined,
  fileRepositoryName: string | undefined,
  changeLayer?: GitChangeLayer,
): string | undefined {
  return useMemo(
    () =>
      mode === "file" && filePath
        ? reviewFileKey({
            path: filePath,
            repository_name: fileRepositoryName,
            change_layer: changeLayer,
          })
        : undefined,
    [mode, filePath, fileRepositoryName, changeLayer],
  );
}

/** Banner shown when the backend dropped files from a huge cumulative diff
 *  (large rebase) to keep the rendered row count bounded. */
export function TruncatedFilesBanner({ count }: { count: number }) {
  const { t } = useTranslation();
  if (count <= 0) return null;
  return (
    <div
      data-testid="changes-truncated-banner"
      className="flex items-center gap-2 px-4 py-2 text-xs text-yellow-600 bg-yellow-500/10 border-b border-yellow-500/20"
    >
      <IconAlertTriangle className="h-3.5 w-3.5 shrink-0" />
      <span>
        {t("task:changesHiddenBanner", { count, formattedCount: count.toLocaleString() })}
      </span>
    </div>
  );
}
