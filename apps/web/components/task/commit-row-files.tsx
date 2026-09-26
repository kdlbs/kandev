"use client";

import { useMemo } from "react";
import { IconAlertCircle, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useCommitDetail } from "@/hooks/domains/session/use-commit-detail";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTree } from "@/hooks/use-tree";
import type { FileInfo } from "@/lib/state/store";
import type { CommitDetailTarget } from "./changes-diff-target";
import type { ChangedFile } from "./changes-panel-helpers";
import { buildChangesTree, type ChangesTreeNode } from "./changes-file-tree-model";
import { FileRow } from "./changes-panel-file-row";
import { TreeDirRow } from "./changes-panel-tree";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type CommitRowFilesProps = {
  target: CommitDetailTarget;
  onOpenFile?: (path: string) => void;
};

function toChangedFile(path: string, file: FileInfo, target: CommitDetailTarget): ChangedFile {
  return {
    path,
    status: file.status,
    staged: false,
    plus: file.additions,
    minus: file.deletions,
    oldPath: file.old_path,
    isSymlink: file.is_symlink,
    repositoryName:
      target.source === "local" ? target.repo : (target.repositoryName ?? target.repo),
  };
}

const noMutation = () => {};

function CommitFileRow({
  file,
  onOpenFile,
  treeMode = false,
  indentPx,
}: {
  file: ChangedFile;
  onOpenFile?: (path: string) => void;
  treeMode?: boolean;
  indentPx?: number;
}) {
  return (
    <FileRow
      file={file}
      isPending={false}
      readOnly
      testId={`commit-file-${file.path.replace(/[/\\]/g, "-")}`}
      onOpenDiff={(path) => onOpenFile?.(path)}
      onStage={noMutation}
      onUnstage={noMutation}
      onDiscard={noMutation}
      onEditFile={noMutation}
      treeMode={treeMode}
      indentPx={indentPx}
    />
  );
}

function CommitRowTree({
  files,
  onOpenFile,
}: {
  files: ChangedFile[];
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
          <TreeDirRow
            key={row.path}
            row={row}
            baseIndentPx={0}
            onToggle={() => toggle(row.path)}
            testId={`commit-file-tree-dir-${row.path.replace(/[/\\]/g, "-")}`}
          />
        ) : (
          <CommitFileRow
            key={row.path}
            file={row.node.file!}
            treeMode
            indentPx={row.depth * 12}
            onOpenFile={onOpenFile}
          />
        ),
      )}
    </ul>
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
    return <CommitRowTree files={changedFiles} onOpenFile={onOpenFile} />;
  }
  return (
    <ul data-testid="commit-file-list-inline" className="space-y-0.5">
      {changedFiles.map((file) => (
        <CommitFileRow key={file.path} file={file} onOpenFile={onOpenFile} />
      ))}
    </ul>
  );
}
