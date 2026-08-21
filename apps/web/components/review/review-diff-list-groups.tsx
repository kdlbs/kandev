"use client";

import { IconGitBranch } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

/**
 * Sticky per-repository section header rendered above each group of files in
 * the changes panel. The grouping logic lives in @/lib/group-by-repo so the
 * Review dialog and the dockview Changes tab share the same Map-based
 * bucketer. Only this presentational header is review-specific.
 */
export function RepoGroupHeader({
  name,
  fileCount,
  isSubmodule = false,
}: {
  name: string;
  fileCount: number;
  isSubmodule?: boolean;
}) {
  const { t } = useTranslation();
  // The empty-name group ("uncategorised" — no repository_name on its files)
  // gets a generic label so the user understands what they're looking at.
  const label = name || t("task:otherChanges");
  // Keep h-8 aligned with ReviewDiffHeader's top-8 and grouped sections' scroll-mt-8.
  return (
    <div
      className="sticky top-0 z-20 flex h-8 items-center gap-2 border-y border-border/60 bg-muted/40 px-4 text-xs font-medium text-foreground backdrop-blur-sm"
      data-testid="changes-repo-header"
      data-submodule-scope={isSubmodule ? "true" : undefined}
      aria-label={isSubmodule ? t("common:reviewSubmoduleBoundary", { scope: name }) : undefined}
    >
      {isSubmodule && (
        <IconGitBranch aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-primary" />
      )}
      <span className="truncate">{label}</span>
      <span className="text-muted-foreground/70">
        {fileCount} {fileCount === 1 ? "file" : "files"}
      </span>
    </div>
  );
}
