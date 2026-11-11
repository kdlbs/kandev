"use client";

import { IconGitBranch } from "@tabler/icons-react";
import type { TaskRepositoryRoot } from "@/hooks/domains/session/use-task-repository-roots";

export function FileTreeRootBadge({ root }: { root: TaskRepositoryRoot }) {
  return (
    <span
      className="ml-1 flex min-w-0 shrink items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground"
      data-testid="file-tree-root-branch"
      title={root.branch}
    >
      <IconGitBranch className="h-3 w-3 shrink-0" />
      <span className="truncate">{root.branch || root.repositoryName}</span>
    </span>
  );
}
