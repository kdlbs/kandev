"use client";

import { IconAlertCircle, IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { groupByRepositoryName } from "@/lib/group-by-repo";
import type { CommitDetailTarget, OpenDiffOptions } from "@/lib/state/diff-target-types";
import type { CommitItem } from "./commit-row";
import { CommitRow, createCommitFileNavigationRequest } from "./commit-row";
import { CommitFileRow } from "./commit-row-files";
import { PRFileRow } from "./changes-panel-pr-files";
import { CommitsGroupActions } from "./changes-panel-repo-groups";
import type { ChangedFile } from "./changes-panel-helpers";
import type { ChangesHistoryTimelineRow } from "./changes-timeline-model";
import { TreeDirRow } from "./changes-panel-tree";
import type { CommitInlineFile } from "./changes-inline-commit-state";
import type { PRChangedFile } from "./changes-panel-timeline";

type HistoryRowActions = {
  onOpenDiff: (path: string, options?: OpenDiffOptions) => void;
  onOpenCommitDetail?: (
    target: CommitDetailTarget,
    fileNavigation?: { path: string; token: number },
  ) => void;
  onRevertCommit?: (sha: string, repo?: string) => void;
  onAmendCommit?: (message: string, repo?: string) => void;
  onResetToCommit?: (sha: string, repo?: string) => void;
  onRepoPush?: (repo: string) => void;
  onRepoCreatePR?: (repo: string) => void;
  repoDisplayName?: (repositoryName: string) => string | undefined;
  perRepoStatus?: Array<{ repository_name: string; pushAhead?: number; ahead?: number }>;
  prByRepo?: Record<string, string | undefined>;
  pushDisabled: boolean;
};

export function ChangesTimelineHistoryRow({
  row,
  sectionCommits = [],
  actions,
  onToggleSection,
  onToggleRepository,
  onToggleCommit,
  onCommitTargetMounted,
  onRetryCommit,
  onToggleInlineDirectory,
}: ChangesTimelineHistoryRowProps) {
  switch (row.kind) {
    case "history-section":
      return (
        <HistorySectionRow
          row={row}
          sectionCommits={sectionCommits}
          actions={actions}
          onToggleSection={onToggleSection}
        />
      );
    case "history-repository":
      return (
        <HistoryRepositoryRow row={row} actions={actions} onToggleRepository={onToggleRepository} />
      );
    case "pr-file":
      return <PRHistoryRow row={row} onOpenDiff={actions.onOpenDiff} />;
    case "commit":
      return (
        <CommitHistoryRow
          row={row}
          actions={actions}
          onToggleCommit={onToggleCommit}
          onCommitTargetMounted={onCommitTargetMounted}
        />
      );
    case "commit-directory":
      return (
        <CommitDirectoryHistoryRow row={row} onToggleInlineDirectory={onToggleInlineDirectory} />
      );
    case "commit-file":
      return <CommitFileHistoryRow row={row} actions={actions} />;
    case "commit-status":
      return <CommitStatusHistoryRow row={row} onRetryCommit={onRetryCommit} />;
  }
}

type ChangesTimelineHistoryRowProps = {
  row: ChangesHistoryTimelineRow;
  sectionCommits?: CommitItem[];
  actions: HistoryRowActions;
  onToggleSection: (sectionKey: string) => void;
  onToggleRepository: (sectionKey: string, repositoryName: string) => void;
  onToggleCommit: (sectionKey: string, target: CommitDetailTarget, expanded: boolean) => void;
  onCommitTargetMounted?: (targetKey: string) => () => void;
  onRetryCommit: (target: CommitDetailTarget) => void;
  onToggleInlineDirectory: (sectionKey: string, target: CommitDetailTarget, path: string) => void;
};

function HistorySectionRow({
  row,
  sectionCommits,
  actions,
  onToggleSection,
}: {
  row: Extract<ChangesHistoryTimelineRow, { kind: "history-section" }>;
  sectionCommits: CommitItem[];
  actions: HistoryRowActions;
  onToggleSection: ChangesTimelineHistoryRowProps["onToggleSection"];
}) {
  const aheadByRepo = new Map(
    (actions.perRepoStatus ?? []).map((status) => [
      status.repository_name,
      status.pushAhead ?? status.ahead ?? 0,
    ]),
  );

  const groups = groupByRepositoryName(sectionCommits, (commit) => commit.repository_name);
  const singleUnscopedRepo = groups.length === 1 && groups[0]?.repositoryName === "";
  const canShowActions = row.historyKind === "commits" && row.showActions && singleUnscopedRepo;
  const sectionAction = canShowActions ? (
    <CommitsGroupActions
      repositoryName=""
      aheadCount={aheadByRepo.get("") ?? 0}
      prExists={!!actions.prByRepo?.[""]}
      canCreatePR={!!actions.onRepoCreatePR && !actions.prByRepo?.[""]}
      onRepoPush={!actions.pushDisabled ? actions.onRepoPush : undefined}
      onRepoCreatePR={actions.onRepoCreatePR}
      stop={(event) => event.stopPropagation()}
    />
  ) : undefined;
  const toggleTestId = row.historyKind === "pr" ? "pr-changes-section" : row.testId;
  return (
    <div className="relative flex gap-2.5">
      <div className="flex flex-col items-center">
        <div
          className={`relative z-10 mt-[5px] size-1.5 shrink-0 rounded-full ${row.historyKind === "pr" ? "bg-purple-500" : "bg-blue-500"}`}
        />
      </div>
      <div className="min-w-0 flex-1 pb-3">
        <div className="mb-1 flex items-center justify-between gap-2 -mt-0.5">
          <button
            type="button"
            className="flex min-h-6 items-center gap-1 text-[11px] font-medium uppercase tracking-wider text-foreground/70 cursor-pointer hover:text-foreground/90 [@media(pointer:coarse)]:min-h-11"
            onClick={() => onToggleSection(row.sectionKey)}
            aria-expanded={!row.collapsed}
            data-testid={`${toggleTestId}-collapse-toggle`}
            data-changes-row-focus
          >
            {row.label}
            <span className="text-muted-foreground/50 font-normal">({row.count})</span>
            {row.collapsed ? (
              <IconChevronRight className="h-3 w-3 text-muted-foreground/50" />
            ) : (
              <IconChevronDown className="h-3 w-3 text-muted-foreground/50" />
            )}
          </button>
          {sectionAction}
        </div>
      </div>
    </div>
  );
}

function HistoryRepositoryRow({
  row,
  actions,
  onToggleRepository,
}: {
  row: Extract<ChangesHistoryTimelineRow, { kind: "history-repository" }>;
  actions: HistoryRowActions;
  onToggleRepository: ChangesTimelineHistoryRowProps["onToggleRepository"];
}) {
  const { t } = useTranslation();
  const aheadByRepo = new Map(
    (actions.perRepoStatus ?? []).map((status) => [
      status.repository_name,
      status.pushAhead ?? status.ahead ?? 0,
    ]),
  );
  const label =
    actions.repoDisplayName?.(row.repositoryName) || row.repositoryName || t("common:repository");
  const showCommitActions = sectionCommitActionsEnabled(row);
  const prExists = !!(actions.prByRepo?.[row.repositoryName] ?? actions.prByRepo?.[""]);
  return (
    <div className="flex items-center justify-between gap-2 px-1 py-0.5">
      <button
        type="button"
        className="flex min-h-6 min-w-0 items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground/80 cursor-pointer hover:text-foreground/80 [@media(pointer:coarse)]:min-h-11"
        data-testid={row.historyKind === "pr" ? "pr-files-repo-header" : "commits-repo-header"}
        data-changes-row-focus
        aria-expanded={!row.collapsed}
        onClick={() => onToggleRepository(row.sectionKey, row.repositoryName)}
      >
        {row.collapsed ? (
          <IconChevronRight className="h-3 w-3 shrink-0 text-muted-foreground/50" />
        ) : (
          <IconChevronDown className="h-3 w-3 shrink-0 text-muted-foreground/50" />
        )}
        <span className="truncate">{label}</span>
        <span className="text-muted-foreground/50 normal-case tracking-normal">{row.count}</span>
      </button>
      {showCommitActions && (
        <CommitsGroupActions
          repositoryName={row.repositoryName}
          aheadCount={aheadByRepo.get(row.repositoryName) ?? 0}
          prExists={prExists}
          canCreatePR={!!actions.onRepoCreatePR && !prExists}
          onRepoPush={!actions.pushDisabled ? actions.onRepoPush : undefined}
          onRepoCreatePR={actions.onRepoCreatePR}
          stop={(event) => event.stopPropagation()}
        />
      )}
    </div>
  );
}

function PRHistoryRow({
  row,
  onOpenDiff,
}: {
  row: Extract<ChangesHistoryTimelineRow, { kind: "pr-file" }>;
  onOpenDiff: HistoryRowActions["onOpenDiff"];
}) {
  const file: PRChangedFile = {
    path: row.path,
    prKey: row.prKey,
    status: row.status,
    plus: row.plus,
    minus: row.minus,
    oldPath: row.oldPath,
    repository_name: row.repositoryName,
  };
  return (
    <ul className="space-y-0.5">
      <PRHistoryFileRow file={file} onOpenDiff={onOpenDiff} />
    </ul>
  );
}

function CommitHistoryRow({
  row,
  actions,
  onToggleCommit,
  onCommitTargetMounted,
}: {
  row: Extract<ChangesHistoryTimelineRow, { kind: "commit" }>;
  actions: HistoryRowActions;
  onToggleCommit: ChangesTimelineHistoryRowProps["onToggleCommit"];
  onCommitTargetMounted: ChangesTimelineHistoryRowProps["onCommitTargetMounted"];
}) {
  useEffect(() => onCommitTargetMounted?.(row.targetKey), [onCommitTargetMounted, row.targetKey]);
  return (
    <CommitRow
      commit={row.commit}
      isLatest={row.isLatest}
      onOpenCommitDetail={actions.onOpenCommitDetail}
      onRevertCommit={row.commit.pushed ? undefined : actions.onRevertCommit}
      onAmendCommit={row.commit.pushed ? undefined : actions.onAmendCommit}
      onResetToCommit={row.commit.pushed ? undefined : actions.onResetToCommit}
      controlledExpansion={{
        expanded: row.expanded,
        onToggle: () => onToggleCommit(row.sectionKey, row.target, !row.expanded),
      }}
    />
  );
}

function CommitDirectoryHistoryRow({
  row,
  onToggleInlineDirectory,
}: {
  row: Extract<ChangesHistoryTimelineRow, { kind: "commit-directory" }>;
  onToggleInlineDirectory: ChangesTimelineHistoryRowProps["onToggleInlineDirectory"];
}) {
  return (
    <TreeDirRow
      row={row.row}
      baseIndentPx={0}
      onToggle={() => onToggleInlineDirectory(row.sectionKey, row.target, row.row.path)}
      testId={`commit-file-tree-dir-${row.row.path.replace(/[/\\]/g, "-")}`}
    />
  );
}

function CommitFileHistoryRow({
  row,
  actions,
}: {
  row: Extract<ChangesHistoryTimelineRow, { kind: "commit-file" }>;
  actions: HistoryRowActions;
}) {
  const changedFile = inlineFileToChangedFile(row.file);
  return (
    <ul className="space-y-0.5">
      <CommitFileRow
        file={changedFile}
        treeMode={row.treeMode}
        keyboardNavigable
        indentPx={row.indentPx}
        onOpenFile={(path) =>
          actions.onOpenCommitDetail?.(row.target, createCommitFileNavigationRequest(path))
        }
      />
    </ul>
  );
}

function CommitStatusHistoryRow({
  row,
  onRetryCommit,
}: {
  row: Extract<ChangesHistoryTimelineRow, { kind: "commit-status" }>;
  onRetryCommit: ChangesTimelineHistoryRowProps["onRetryCommit"];
}) {
  const { t } = useTranslation();
  if (row.status === "empty") {
    return (
      <div className="px-3 py-3 text-xs text-muted-foreground">{t("task:noFilesInThisCommit")}</div>
    );
  }
  return (
    <div role="alert" className="flex items-center gap-2 px-3 py-3 text-xs text-destructive">
      <IconAlertCircle className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1">{row.error}</span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
        onClick={() => onRetryCommit(row.target)}
      >
        {t("system:featureTogglesRetry")}
      </Button>
    </div>
  );
}

function sectionCommitActionsEnabled(
  row: Extract<ChangesHistoryTimelineRow, { kind: "history-repository" }>,
): boolean {
  return row.historyKind === "commits" && row.showActions;
}

function PRHistoryFileRow({
  file,
  onOpenDiff,
}: {
  file: PRChangedFile;
  onOpenDiff: HistoryRowActions["onOpenDiff"];
}) {
  const activeFilePath = useDockviewStore((state) => state.activeFilePath);
  return <PRFileRow file={file} onOpenDiff={onOpenDiff} isActive={file.path === activeFilePath} />;
}

function inlineFileToChangedFile(file: CommitInlineFile): ChangedFile {
  return {
    path: file.path,
    status: file.status,
    staged: false,
    plus: file.plus,
    minus: file.minus,
    oldPath: file.oldPath,
    isSymlink: file.isSymlink,
    repositoryName: file.repositoryName,
  };
}
