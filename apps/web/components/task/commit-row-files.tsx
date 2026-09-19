"use client";

import { useMemo } from "react";
import {
  IconAlertCircle,
  IconChevronDown,
  IconChevronRight,
  IconLoader2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { FileStatusIcon } from "@/components/shared/file-status-icon";
import { useAppStore } from "@/components/state-provider";
import { splitCollapsibleFilePath } from "@/components/diff/collapsible-file-header";
import { useCommitDetail } from "@/hooks/domains/session/use-commit-detail";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTree, type VisibleRow } from "@/hooks/use-tree";
import type { FileInfo } from "@/lib/state/store";
import type { CommitDetailTarget } from "./changes-diff-target";
import type { ChangedFile } from "./changes-panel-helpers";
import { buildChangesTree, type ChangesTreeNode } from "./changes-file-tree-model";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type CommitRowFilesProps = {
  target: CommitDetailTarget;
  onOpenFile?: (path: string) => void;
};

function fileStats(file: FileInfo, unavailable: string) {
  return (
    <span className="shrink-0 whitespace-nowrap text-[11px] tabular-nums text-muted-foreground">
      <span className="text-emerald-500">
        +{typeof file.additions === "number" ? file.additions : unavailable}
      </span>
      <span aria-hidden="true"> / </span>
      <span className="text-rose-500">
        -{typeof file.deletions === "number" ? file.deletions : unavailable}
      </span>
    </span>
  );
}

function toChangedFile(path: string, file: FileInfo, target: CommitDetailTarget): ChangedFile {
  return {
    path,
    status: file.status,
    staged: false,
    plus: file.additions,
    minus: file.deletions,
    oldPath: file.old_path,
    repositoryName:
      target.source === "local" ? target.repo : (target.repositoryName ?? target.repo),
  };
}

function CommitRowFileButton({
  path,
  file,
  indentPx = 0,
  onOpenFile,
}: {
  path: string;
  file: FileInfo;
  indentPx?: number;
  onOpenFile?: (path: string) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const { directory, name } = splitCollapsibleFilePath(path);
  return (
    <button
      type="button"
      data-testid={`commit-file-${path.replace(/[/\\]/g, "-")}`}
      data-file-path={path}
      aria-label={path}
      title={path}
      className="flex min-h-7 w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-left text-xs hover:bg-muted/60 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      style={{ paddingLeft: indentPx + 8 }}
      onClick={() => onOpenFile?.(path)}
    >
      <FileStatusIcon status={file.status} oldPath={file.old_path} className="size-3.5" />
      <span className="min-w-0 flex-1">
        {isMobile ? (
          <span className="flex min-w-0 flex-col justify-center leading-4">
            <span className="truncate font-medium text-foreground">{name}</span>
            {directory && (
              <span className="truncate text-[11px] text-muted-foreground">{directory}</span>
            )}
          </span>
        ) : (
          <span className="block truncate">{path}</span>
        )}
      </span>
      {fileStats(file, t("common:unavailable"))}
    </button>
  );
}

function CommitRowTree({
  files,
  fileByPath,
  onOpenFile,
}: {
  files: ChangedFile[];
  fileByPath: ReadonlyMap<string, FileInfo>;
  onOpenFile?: (path: string) => void;
}) {
  const tree = useMemo(() => buildChangesTree(files), [files]);
  const { visibleRows, toggle } = useTree<ChangesTreeNode>({
    nodes: tree,
    getPath: (node) => node.path,
    getChildren: (node) => node.children,
    isDir: (node) => node.isDir,
    defaultExpanded: "all",
    chainCollapse: true,
  });
  return (
    <ul data-testid="commit-file-tree-inline" className="space-y-0.5">
      {visibleRows.map((row) =>
        row.isDir ? (
          <CommitRowTreeDirectory key={row.path} row={row} onToggle={() => toggle(row.path)} />
        ) : (
          (() => {
            const file = fileByPath.get(row.path);
            if (!file) return null;
            return (
              <li key={row.path} data-testid="commit-file-entry">
                <CommitRowFileButton
                  path={row.path}
                  file={file}
                  indentPx={row.depth * 12}
                  onOpenFile={onOpenFile}
                />
              </li>
            );
          })()
        ),
      )}
    </ul>
  );
}

function CommitRowTreeDirectory({
  row,
  onToggle,
}: {
  row: VisibleRow<ChangesTreeNode>;
  onToggle: () => void;
}) {
  return (
    <li>
      <button
        type="button"
        data-testid={`commit-file-tree-dir-${row.path.replace(/[/\\]/g, "-")}`}
        aria-expanded={row.isExpanded}
        className="flex min-h-7 w-full cursor-pointer items-center gap-1 rounded-md px-2 text-left text-xs text-foreground/70 hover:bg-muted/60 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        style={{ paddingLeft: row.depth * 12 + 8 }}
        onClick={onToggle}
      >
        {row.isExpanded ? (
          <IconChevronDown className="size-3 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronRight className="size-3 shrink-0 text-muted-foreground" />
        )}
        <span className="truncate">{row.displayName}</span>
      </button>
    </li>
  );
}

export function CommitRowFiles({ target, onOpenFile }: CommitRowFilesProps) {
  const { t } = useTranslation();
  const layout = useAppStore((state) => state.userSettings.changesPanelLayout);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touchSized = isMobile || isFinePointer === false;
  const { files, loading, error, refetch } = useCommitDetail(target);
  const entries = useMemo(
    () => (files ? Object.entries(files).sort(([left], [right]) => left.localeCompare(right)) : []),
    [files],
  );
  const changedFiles = useMemo(
    () => entries.map(([path, file]) => toChangedFile(path, file, target)),
    [entries, target],
  );
  const fileByPath = useMemo(() => new Map(entries), [entries]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 px-3 py-3 text-xs text-muted-foreground">
        <IconLoader2 className="size-3.5 animate-spin" />
        {t("task:loadingFiles")}
      </div>
    );
  }
  if (error) {
    return (
      <div role="alert" className="flex items-center gap-2 px-3 py-3 text-xs text-destructive">
        <IconAlertCircle className="size-3.5 shrink-0" />
        <span className="min-w-0 flex-1">{error}</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={cn("cursor-pointer", touchSized && "min-h-11")}
          onClick={() => void refetch()}
        >
          {t("system:featureTogglesRetry")}
        </Button>
      </div>
    );
  }
  if (entries.length === 0) {
    return (
      <div className="px-3 py-3 text-xs text-muted-foreground">{t("task:noFilesInThisCommit")}</div>
    );
  }

  if (layout === "tree") {
    return <CommitRowTree files={changedFiles} fileByPath={fileByPath} onOpenFile={onOpenFile} />;
  }
  return (
    <ul data-testid="commit-file-list-inline" className="space-y-0.5">
      {entries.map(([path, file]) => (
        <li key={path} data-testid="commit-file-entry">
          <CommitRowFileButton path={path} file={file} onOpenFile={onOpenFile} />
        </li>
      ))}
    </ul>
  );
}
