"use client";

import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useMultiSelect } from "@/hooks/use-multi-select";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStore } from "@/components/state-provider";
import { Button } from "@kandev/ui/button";
import type { OpenDiffOptions } from "@/lib/state/diff-target-types";
import { FileRow, BulkActionTargetBar } from "./changes-panel-file-row";
import { FileSectionActions } from "./changes-panel-repo-groups";
import { TreeDirRow } from "./changes-panel-tree";
import type { ChangedFile } from "./changes-panel-helpers";
import {
  buildChangesTimelineRows,
  changesDirectoryExpansionKey,
  changesRepositoryExpansionKey,
  type ChangesHistoryTimelineRow,
  type ChangesTimelineRow,
  type ChangesTimelineSectionInput,
} from "./changes-timeline-model";
import {
  changedFileTargetsFromSelectionKeys,
  changesFileSelectionKey,
  type ChangedFileTarget,
  type ChangesFileSection,
} from "./changes-timeline-selection";
import {
  ChangesTimelineViewport,
  type ChangesTimelineFocusRequest,
  type ChangesTimelineGroup,
} from "./changes-timeline-viewport";

const SECTION_DOT_COLORS: Record<ChangesFileSection, string> = {
  unstaged: "bg-yellow-500",
  staged: "bg-emerald-500",
};

type ChangesWorkingTreeProps = {
  hasUnstaged: boolean;
  hasStaged: boolean;
  unstagedFiles: ChangedFile[];
  stagedFiles: ChangedFile[];
  pendingStageFiles: Set<string>;
  onOpenDiff: (path: string, options?: OpenDiffOptions) => void;
  onEditFile: (path: string, repo?: string) => void;
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
  onDiscard: (path: string, repo?: string, anchor?: HTMLElement) => void;
  onBulkStage: (files: ChangedFileTarget[]) => void;
  onBulkUnstage: (files: ChangedFileTarget[]) => void;
  onBulkDiscard: (files: ChangedFileTarget[], anchor?: HTMLElement) => void;
  onRepoStageAll?: (repositoryName: string) => void;
  onRepoUnstageAll?: (repositoryName: string) => void;
  onRepoCommit?: (repositoryName: string) => void;
  repoDisplayName?: (repositoryName: string) => string | undefined;
  isLoading: boolean;
  scrollElement: HTMLDivElement | null;
  beforeLayoutKey: string;
  contextKey?: string;
  focusRequest?: ChangesTimelineFocusRequest;
  historyRowsBefore?: ChangesHistoryTimelineRow[];
  historyRowsAfter?: ChangesHistoryTimelineRow[];
  renderHistoryRow?: (row: ChangesHistoryTimelineRow) => React.ReactNode;
};

export function ChangesWorkingTree(props: ChangesWorkingTreeProps) {
  return <ChangesWorkingTreeContext key={props.contextKey ?? ""} props={props} />;
}

function ChangesWorkingTreeContext({ props }: { props: ChangesWorkingTreeProps }) {
  const { t } = useTranslation();
  const layout = useAppStore((state) => state.userSettings.changesPanelLayout);
  const activeFilePath = useDockviewStore((state) => state.activeFilePath);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touchMode = isMobile || !isFinePointer;
  const disclosure = useWorkingTreeDisclosureState();
  const selection = useWorkingTreeSelection(props);
  const rows = useWorkingTreeRows(props, layout, disclosure, selection);
  const { anchorRef, scrollMargin } = useWorkingTreeScrollMargin(props);
  const clearSelectionOnEscape = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (event.key !== "Escape") return;
      selection.unstagedSelection.clearSelection();
      selection.stagedSelection.clearSelection();
    },
    [selection.stagedSelection, selection.unstagedSelection],
  );
  const estimateSize = useCallback(
    (row: ChangesTimelineRow) => estimateChangesTimelineRowSize(row, touchMode),
    [touchMode],
  );

  return (
    <div ref={anchorRef} className="min-w-0" onKeyDown={clearSelectionOnEscape}>
      <ChangesTimelineViewport
        rows={rows}
        scrollElement={props.scrollElement}
        contextKey={props.contextKey}
        focusRequest={props.focusRequest}
        scrollMargin={scrollMargin}
        bottomPadding={12}
        estimateSize={estimateSize}
        getGroups={getTimelineGroups}
        renderRow={(row) =>
          renderChangesTimelineRow({
            row,
            props,
            activeFilePath,
            t,
            ...selection,
            ...disclosure,
            renderHistoryRow: props.renderHistoryRow,
          })
        }
      />
    </div>
  );
}

type WorkingTreeDisclosureState = {
  collapsedSections: Set<ChangesFileSection>;
  collapsedRepositories: Set<string>;
  collapsedDirectories: Set<string>;
  toggleSection: (variant: ChangesFileSection) => void;
  toggleRepository: (key: string) => void;
  toggleDirectory: (key: string) => void;
};

function useWorkingTreeDisclosureState(): WorkingTreeDisclosureState {
  const [collapsedSections, setCollapsedSections] = useState<Set<ChangesFileSection>>(
    () => new Set(),
  );
  const [collapsedRepositories, setCollapsedRepositories] = useState<Set<string>>(() => new Set());
  const [collapsedDirectories, setCollapsedDirectories] = useState<Set<string>>(() => new Set());
  const toggleSection = useCallback((variant: ChangesFileSection) => {
    setCollapsedSections((current) => toggleSetValue(current, variant));
  }, []);
  const toggleRepository = useCallback((key: string) => {
    setCollapsedRepositories((current) => toggleSetValue(current, key));
  }, []);
  const toggleDirectory = useCallback((key: string) => {
    setCollapsedDirectories((current) => toggleSetValue(current, key));
  }, []);

  return {
    collapsedSections,
    collapsedRepositories,
    collapsedDirectories,
    toggleSection,
    toggleRepository,
    toggleDirectory,
  };
}

type WorkingTreeSelectionState = {
  unstagedSelection: ReturnType<typeof useMultiSelect>;
  stagedSelection: ReturnType<typeof useMultiSelect>;
  unstagedSelectedKeys: string[];
  stagedSelectedKeys: string[];
};

function useWorkingTreeSelection(props: ChangesWorkingTreeProps): WorkingTreeSelectionState {
  const unstagedSelectionItems = useMemo(
    () => props.unstagedFiles.map((file) => changesFileSelectionKey("unstaged", file)),
    [props.unstagedFiles],
  );
  const stagedSelectionItems = useMemo(
    () => props.stagedFiles.map((file) => changesFileSelectionKey("staged", file)),
    [props.stagedFiles],
  );
  const unstagedSelection = useMultiSelect({ items: unstagedSelectionItems });
  const stagedSelection = useMultiSelect({ items: stagedSelectionItems });
  const unstagedSelectedKeys = useMemo(
    () => [...unstagedSelection.selectedPaths],
    [unstagedSelection.selectedPaths],
  );
  const stagedSelectedKeys = useMemo(
    () => [...stagedSelection.selectedPaths],
    [stagedSelection.selectedPaths],
  );
  return { unstagedSelection, stagedSelection, unstagedSelectedKeys, stagedSelectedKeys };
}

function buildWorkingTreeSections(
  props: ChangesWorkingTreeProps,
  collapsedSections: ReadonlySet<ChangesFileSection>,
  selection: Pick<WorkingTreeSelectionState, "unstagedSelectedKeys" | "stagedSelectedKeys">,
): ChangesTimelineSectionInput[] {
  const sections: ChangesTimelineSectionInput[] = [];
  if (props.hasUnstaged) {
    sections.push({
      variant: "unstaged",
      files: props.unstagedFiles,
      collapsed: collapsedSections.has("unstaged"),
      selectedKeys: selection.unstagedSelectedKeys,
    });
  }
  if (props.hasStaged) {
    sections.push({
      variant: "staged",
      files: props.stagedFiles,
      collapsed: collapsedSections.has("staged"),
      selectedKeys: selection.stagedSelectedKeys,
    });
  }
  return sections;
}

function useWorkingTreeRows(
  props: ChangesWorkingTreeProps,
  layout: string,
  disclosure: WorkingTreeDisclosureState,
  selection: WorkingTreeSelectionState,
): ChangesTimelineRow[] {
  const workingRows = useMemo(
    () =>
      buildChangesTimelineRows({
        layout: layout === "tree" ? "tree" : "flat",
        sections: buildWorkingTreeSections(props, disclosure.collapsedSections, selection),
        collapsedRepositories: disclosure.collapsedRepositories,
        collapsedDirectories: disclosure.collapsedDirectories,
      }),
    [
      layout,
      props.hasUnstaged,
      props.unstagedFiles,
      props.hasStaged,
      props.stagedFiles,
      disclosure.collapsedSections,
      disclosure.collapsedRepositories,
      disclosure.collapsedDirectories,
      selection.unstagedSelectedKeys,
      selection.stagedSelectedKeys,
    ],
  );
  return useMemo(
    () => [...(props.historyRowsBefore ?? []), ...workingRows, ...(props.historyRowsAfter ?? [])],
    [props.historyRowsBefore, props.historyRowsAfter, workingRows],
  );
}

function useWorkingTreeScrollMargin(props: ChangesWorkingTreeProps) {
  const [scrollMargin, setScrollMargin] = useState(0);
  const anchorRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const anchor = anchorRef.current;
    const scrollElement = props.scrollElement;
    if (!anchor || !scrollElement) return;

    const updateScrollMargin = () => {
      const top =
        anchor.getBoundingClientRect().top -
        scrollElement.getBoundingClientRect().top +
        scrollElement.scrollTop;
      const next = Math.max(0, Math.round(top));
      setScrollMargin((current) => (current === next ? current : next));
    };
    const observer = new ResizeObserver(updateScrollMargin);
    observer.observe(scrollElement);
    let previous = anchor.previousElementSibling;
    while (previous) {
      observer.observe(previous);
      previous = previous.previousElementSibling;
    }
    window.addEventListener("resize", updateScrollMargin);
    updateScrollMargin();
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", updateScrollMargin);
    };
  }, [props.scrollElement, props.beforeLayoutKey]);
  return { anchorRef, scrollMargin };
}

const DESKTOP_ROW_SIZES: Record<ChangesTimelineRow["kind"], number> = {
  section: 36,
  repository: 28,
  directory: 24,
  file: 28,
  selection: 32,
  "history-section": 36,
  "history-repository": 28,
  commit: 34,
  "commit-directory": 28,
  "commit-file": 28,
  "pr-file": 28,
  "commit-status": 36,
};

const TOUCH_ROW_SIZES: Record<ChangesTimelineRow["kind"], number> = {
  section: 36,
  repository: 44,
  directory: 44,
  file: 48,
  selection: 48,
  "history-section": 36,
  "history-repository": 44,
  commit: 56,
  "commit-directory": 44,
  "commit-file": 48,
  "pr-file": 48,
  "commit-status": 48,
};

function estimateChangesTimelineRowSize(row: ChangesTimelineRow, touchMode: boolean): number {
  return (touchMode ? TOUCH_ROW_SIZES : DESKTOP_ROW_SIZES)[row.kind];
}

function toggleSetValue<T>(current: Set<T>, value: T): Set<T> {
  const next = new Set(current);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

type ChangesTimelineRowRenderArgs = {
  row: ChangesTimelineRow;
  props: ChangesWorkingTreeProps;
  activeFilePath: string | null;
  t: ReturnType<typeof useTranslation>["t"];
  unstagedSelection: WorkingTreeSelectionState["unstagedSelection"];
  stagedSelection: WorkingTreeSelectionState["stagedSelection"];
  toggleSection: WorkingTreeDisclosureState["toggleSection"];
  toggleRepository: WorkingTreeDisclosureState["toggleRepository"];
  toggleDirectory: WorkingTreeDisclosureState["toggleDirectory"];
  renderHistoryRow?: (row: ChangesHistoryTimelineRow) => React.ReactNode;
};

function renderChangesTimelineRow(args: ChangesTimelineRowRenderArgs) {
  const { row } = args;
  if ("groups" in row) return args.renderHistoryRow?.(row) ?? null;
  switch (row.kind) {
    case "section":
      return renderSectionTimelineRow(row, args);
    case "repository":
      return renderRepositoryTimelineRow(row, args);
    case "directory":
      return renderDirectoryTimelineRow(row, args.toggleDirectory);
    case "file":
      return renderFileTimelineRow(row, args);
    case "selection":
      return renderSelectionTimelineRow(row, args.props);
  }
}

function renderSectionTimelineRow(
  row: Extract<ChangesTimelineRow, { kind: "section" }>,
  args: ChangesTimelineRowRenderArgs,
) {
  const isUnstaged = row.variant === "unstaged";
  const label = args.t(isUnstaged ? "task:unstagedFiles" : "task:stagedFiles");
  return (
    <div className="relative flex gap-2.5">
      <div className="flex flex-col items-center">
        <div
          className={`relative z-10 size-1.5 shrink-0 rounded-full mt-[5px] ${SECTION_DOT_COLORS[row.variant]}`}
        />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between gap-2 -mt-0.5 mb-1">
          <button
            type="button"
            className="flex min-h-11 items-center gap-1 text-[11px] font-medium uppercase tracking-wider text-foreground/70 cursor-pointer hover:text-foreground/90 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
            onClick={() => args.toggleSection(row.variant)}
            aria-expanded={!row.collapsed}
            data-testid={`${row.variant}-files-section-collapse-toggle`}
            data-changes-row-focus
          >
            {label}
            <span className="text-muted-foreground/50 font-normal">({row.count})</span>
            {row.collapsed ? (
              <IconChevronRight className="h-3 w-3 text-muted-foreground/50" />
            ) : (
              <IconChevronDown className="h-3 w-3 text-muted-foreground/50" />
            )}
          </button>
          {row.singleRepository && (
            <FileSectionActions
              primaryLabel={isUnstaged ? args.t("task:stageAll") : args.t("task:commit")}
              secondaryLabel={isUnstaged ? undefined : args.t("task:unstageAll")}
              onAction={isUnstaged ? args.props.onRepoStageAll : args.props.onRepoCommit}
              onSecondaryAction={isUnstaged ? undefined : args.props.onRepoUnstageAll}
              disabled={args.props.isLoading}
            />
          )}
        </div>
      </div>
    </div>
  );
}

function renderRepositoryTimelineRow(
  row: Extract<ChangesTimelineRow, { kind: "repository" }>,
  args: ChangesTimelineRowRenderArgs,
) {
  const { props } = args;
  const isUnstaged = row.variant === "unstaged";
  const isStaged = row.variant === "staged";
  return (
    <ChangesRepositoryTimelineRow
      row={row}
      displayName={props.repoDisplayName?.(row.repositoryName) ?? args.t("common:repository")}
      primaryLabel={isUnstaged ? args.t("task:stageAll") : args.t("task:commit")}
      secondaryLabel={isStaged ? args.t("task:unstageAll") : undefined}
      onPrimaryAction={isUnstaged ? props.onRepoStageAll : props.onRepoCommit}
      onSecondaryAction={isStaged ? props.onRepoUnstageAll : undefined}
      disabled={props.isLoading}
      onToggle={() =>
        args.toggleRepository(changesRepositoryExpansionKey(row.variant, row.repositoryName))
      }
    />
  );
}

function renderDirectoryTimelineRow(
  row: Extract<ChangesTimelineRow, { kind: "directory" }>,
  toggleDirectory: ChangesTimelineRowRenderArgs["toggleDirectory"],
) {
  return (
    <TreeDirRow
      row={row.row}
      baseIndentPx={row.nested ? 12 : 0}
      onToggle={() =>
        toggleDirectory(changesDirectoryExpansionKey(row.variant, row.repositoryName, row.row.path))
      }
    />
  );
}

function renderFileTimelineRow(
  row: Extract<ChangesTimelineRow, { kind: "file" }>,
  args: ChangesTimelineRowRenderArgs,
) {
  const selection = row.variant === "unstaged" ? args.unstagedSelection : args.stagedSelection;
  const selectionKey = changesFileSelectionKey(row.variant, row.file);
  return (
    <FileRow
      file={row.file}
      isPending={args.props.pendingStageFiles.has(
        `${row.file.repositoryName ?? ""}::${row.file.path}`,
      )}
      isSelected={selection.isSelected(selectionKey)}
      isActive={row.file.path === args.activeFilePath}
      onSelect={(_path, event) => selection.handleClick(selectionKey, event)}
      onOpenDiff={args.props.onOpenDiff}
      keyboardNavigable
      onStage={args.props.onStage}
      onUnstage={args.props.onUnstage}
      onDiscard={args.props.onDiscard}
      onEditFile={args.props.onEditFile}
      treeMode={row.treeMode}
      indentPx={row.indentPx}
    />
  );
}

function renderSelectionTimelineRow(
  row: Extract<ChangesTimelineRow, { kind: "selection" }>,
  props: ChangesWorkingTreeProps,
) {
  const targets = changedFileTargetsFromSelectionKeys(row.selectedKeys);
  return (
    <div className="mt-1.5 flex items-center gap-1.5">
      <BulkActionTargetBar
        variant={row.variant}
        selectionCount={targets.length}
        selectedFiles={targets}
        onBulkStage={props.onBulkStage}
        onBulkUnstage={props.onBulkUnstage}
        onBulkDiscard={props.onBulkDiscard}
      />
    </div>
  );
}

function ChangesRepositoryTimelineRow({
  row,
  displayName,
  primaryLabel,
  secondaryLabel,
  onPrimaryAction,
  onSecondaryAction,
  disabled,
  onToggle,
}: {
  row: Extract<ChangesTimelineRow, { kind: "repository" }>;
  displayName: string;
  primaryLabel: string;
  secondaryLabel?: string;
  onPrimaryAction?: (repositoryName: string) => void;
  onSecondaryAction?: (repositoryName: string) => void;
  disabled: boolean;
  onToggle: () => void;
}) {
  const stop = (event: React.MouseEvent) => event.stopPropagation();
  return (
    <div className={row.treeMode ? "-ml-2" : ""}>
      <div className="flex items-center justify-between gap-2 px-1 py-0.5">
        <button
          type="button"
          className="flex min-h-11 min-w-0 items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground/80 cursor-pointer hover:text-foreground/80 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          data-testid="changes-repo-header"
          data-changes-row-focus
          aria-expanded={!row.collapsed}
          onClick={onToggle}
        >
          {row.collapsed ? (
            <IconChevronRight className="h-3 w-3 shrink-0 text-muted-foreground/50" />
          ) : (
            <IconChevronDown className="h-3 w-3 shrink-0 text-muted-foreground/50" />
          )}
          <span className="truncate">{displayName}</span>
          <span className="text-muted-foreground/50 normal-case tracking-normal">{row.count}</span>
        </button>
        {(onPrimaryAction || onSecondaryAction) && (
          <div className="flex items-center gap-1" onClick={stop}>
            {onPrimaryAction && (
              <Button
                size="sm"
                variant="ghost"
                className="h-5 min-h-11 px-1.5 text-[10px] cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
                data-testid="repo-group-action"
                disabled={disabled}
                onClick={() => onPrimaryAction(row.repositoryName)}
              >
                {primaryLabel}
              </Button>
            )}
            {onSecondaryAction && secondaryLabel && (
              <Button
                size="sm"
                variant="ghost"
                className="h-5 min-h-11 px-1.5 text-[10px] text-muted-foreground cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
                data-testid="repo-group-secondary-action"
                disabled={disabled}
                onClick={() => onSecondaryAction(row.repositoryName)}
              >
                {secondaryLabel}
              </Button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function getSectionGroup(
  row: Extract<ChangesTimelineRow, { variant: ChangesFileSection }>,
): ChangesTimelineGroup {
  return {
    key: `changes-section:${row.variant}`,
    testId: `${row.variant}-files-section`,
  };
}

function getRepositoryGroup(
  row: Extract<ChangesTimelineRow, { variant: ChangesFileSection }>,
): ChangesTimelineGroup | undefined {
  if (row.kind === "repository") {
    return {
      key: row.repositoryGroupKey,
      testId: "changes-repo-group",
      attributes: { "data-repository-name": row.repositoryName || "" },
    };
  }
  if ((row.kind === "directory" || row.kind === "file") && row.repositoryGroupKey) {
    return {
      key: row.repositoryGroupKey,
      testId: "changes-repo-group",
      attributes: { "data-repository-name": row.repositoryName || "" },
    };
  }
  return undefined;
}

function getTimelineGroups(row: ChangesTimelineRow): ChangesTimelineGroup[] {
  if ("groups" in row) return row.groups;
  const section = getSectionGroup(row);
  const repository = getRepositoryGroup(row);
  return repository ? [section, repository] : [section];
}
