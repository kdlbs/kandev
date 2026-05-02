"use client";

import { useState } from "react";
import { IconChevronDown, IconChevronRight, IconGitPullRequest } from "@tabler/icons-react";
import { LineStat } from "@/components/diff-stat";
import { FileStatusIcon } from "./file-status-icon";
import { groupByRepositoryName } from "@/lib/group-by-repo";
import type { PRChangedFile } from "./changes-panel-timeline";

type PRFilesSectionContentProps = {
  files: PRChangedFile[];
  onOpenDiff: (path: string) => void;
  /** Maps a repository_name to a human-readable label for the per-repo header. */
  repoDisplayName?: (repositoryName: string) => string | undefined;
};

/**
 * Body of the PR Changes section: groups files by `repository_name` so
 * multi-repo tasks render one sub-header per repo, mirroring the Commits
 * and Changes sections. Single-repo tasks (one group with empty name)
 * render the file list flat — the section header is enough.
 */
export function PRFilesGroupedList({
  files,
  onOpenDiff,
  repoDisplayName,
}: PRFilesSectionContentProps) {
  const groups = groupByRepositoryName(files, (f) => f.repository_name);
  const showRepoHeaders = groups.length > 1 || (groups[0]?.repositoryName ?? "") !== "";
  return (
    <ul className="space-y-0.5" data-testid="pr-files-list">
      {groups.map((group) => (
        <PRFilesRepoGroup
          key={group.repositoryName || "__no_repo__"}
          repositoryName={group.repositoryName}
          displayName={repoDisplayName?.(group.repositoryName)}
          files={group.items}
          showHeader={showRepoHeaders}
          onOpenDiff={onOpenDiff}
        />
      ))}
    </ul>
  );
}

function PRFilesRepoGroup({
  repositoryName,
  displayName,
  files,
  showHeader,
  onOpenDiff,
}: {
  repositoryName: string;
  displayName?: string;
  files: PRChangedFile[];
  showHeader: boolean;
  onOpenDiff: (path: string) => void;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const label = displayName ?? repositoryName ?? "";
  return (
    <li data-testid="pr-files-repo-group" data-repository-name={repositoryName || ""}>
      {showHeader && (
        <button
          type="button"
          className="flex items-center gap-1.5 px-1 py-0.5 text-[11px] font-medium text-muted-foreground/80 uppercase tracking-wide cursor-pointer hover:text-foreground/80 min-w-0"
          data-testid="pr-files-repo-header"
          aria-expanded={!collapsed}
          onClick={() => setCollapsed((c) => !c)}
        >
          {collapsed ? (
            <IconChevronRight className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          ) : (
            <IconChevronDown className="h-3 w-3 text-muted-foreground/50 shrink-0" />
          )}
          <span className="truncate">{label}</span>
          <span className="text-muted-foreground/50 normal-case tracking-normal">
            {files.length}
          </span>
        </button>
      )}
      {!collapsed && (
        <ul className="space-y-0.5">
          {files.map((file) => (
            <PRFileRow key={file.path} file={file} onOpenDiff={onOpenDiff} />
          ))}
        </ul>
      )}
    </li>
  );
}

function PRFileRow({
  file,
  onOpenDiff,
}: {
  file: PRChangedFile;
  onOpenDiff: (path: string) => void;
}) {
  const lastSlash = file.path.lastIndexOf("/");
  const folder = lastSlash === -1 ? "" : file.path.slice(0, lastSlash);
  const name = lastSlash === -1 ? file.path : file.path.slice(lastSlash + 1);

  return (
    <li
      className="group flex items-center justify-between gap-2 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
      onClick={() => onOpenDiff(file.path)}
    >
      <div className="flex items-center gap-2 min-w-0">
        <div className="flex-shrink-0 flex items-center justify-center size-4">
          <IconGitPullRequest className="h-3 w-3 text-purple-500" />
        </div>
        <button type="button" className="min-w-0 text-left cursor-pointer" title={file.path}>
          <p className="flex text-foreground text-xs min-w-0">
            {folder && <span className="text-foreground/60 truncate shrink">{folder}/</span>}
            <span className="font-medium text-foreground whitespace-nowrap shrink-0">{name}</span>
          </p>
        </button>
      </div>
      <div className="flex items-center gap-2">
        <LineStat added={file.plus} removed={file.minus} />
        <FileStatusIcon status={file.status} />
      </div>
    </li>
  );
}
