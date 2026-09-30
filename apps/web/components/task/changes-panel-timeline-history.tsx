"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Dispatch, MutableRefObject, ReactNode, SetStateAction } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { ChangesInlineCommitState, commitDetailTargetKey } from "./changes-inline-commit-state";
import { ChangesTimelineHistoryRow } from "./changes-timeline-history-row";
import {
  buildChangesHistoryTimelineRows,
  changesCommitExpansionKey,
  changesHistoryRepositoryExpansionKey,
  changesHistorySectionRowKey,
  changesInlineDirectoryExpansionKey,
  type ChangesHistorySectionInput,
  type ChangesHistoryTimelineRow,
} from "./changes-timeline-model";
import {
  firstVisibleSection,
  mergeCommits,
  separateCommitHistories,
} from "./changes-panel-helpers";
import type { CommitItem } from "./commit-row";
import type { ChangesTimelineFocusRequest } from "./changes-timeline-viewport";
import type { ChangesPanelTimelineContentProps } from "./changes-panel-timeline-types";

type ChangesSection = ReturnType<typeof firstVisibleSection>;

export type CommitSection = {
  sectionKey: string;
  label: string;
  testId: string;
  commits: CommitItem[];
  defaultCollapsed: boolean;
  showActions: boolean;
};

export type HistoryExpansionState = {
  defaultCollapsedByKey: Map<string, boolean>;
  sectionOverrides: Map<string, boolean>;
  setSectionOverrides: Dispatch<SetStateAction<Map<string, boolean>>>;
  collapsedRepositories: Set<string>;
  setCollapsedRepositories: Dispatch<SetStateAction<Set<string>>>;
  collapsedInlineDirectories: Set<string>;
  setCollapsedInlineDirectories: Dispatch<SetStateAction<Set<string>>>;
  focusRequest?: ChangesTimelineFocusRequest;
};

export type HistoryRows = {
  before: ChangesHistoryTimelineRow[];
  after: ChangesHistoryTimelineRow[];
};

export function useCommitSections(props: ChangesPanelTimelineContentProps): CommitSection[] {
  const { t } = useTranslation();
  const isDiverged = props.relation.presentation === "separate";
  const separated = isDiverged
    ? separateCommitHistories(props.commits, props.prCommits)
    : { providerCommits: [], localCommits: [] };
  const mergedCommits = isDiverged ? [] : mergeCommits(props.commits, props.prCommits);
  const hasMergedCommits = isDiverged
    ? separated.providerCommits.length > 0 || separated.localCommits.length > 0
    : mergedCommits.length > 0;
  const showCommitsList = props.hasStaged || hasMergedCommits;
  const firstSection = firstVisibleSection({
    hasPRFiles: props.hasPRFiles,
    hasUnstaged: props.hasUnstaged,
    hasStaged: props.hasStaged,
    showCommitsList,
    prFileCount: props.prFiles.length,
  });

  return useMemo(
    () =>
      createCommitSections({
        isDiverged,
        showCommitsList,
        separated,
        mergedCommits,
        firstSection,
        providerPRNumber: props.providerPRNumber,
        t,
      }),
    [
      firstSection,
      isDiverged,
      mergedCommits,
      props.providerPRNumber,
      separated.localCommits,
      separated.providerCommits,
      showCommitsList,
      t,
    ],
  );
}

function createCommitSections({
  isDiverged,
  showCommitsList,
  separated,
  mergedCommits,
  firstSection,
  providerPRNumber,
  t,
}: {
  isDiverged: boolean;
  showCommitsList: boolean;
  separated: ReturnType<typeof separateCommitHistories>;
  mergedCommits: CommitItem[];
  firstSection: ChangesSection;
  providerPRNumber: number | undefined;
  t: TFunction;
}): CommitSection[] {
  if (!showCommitsList) return [];
  if (isDiverged) return createDivergedCommitSections(separated, firstSection, providerPRNumber, t);
  if (mergedCommits.length === 0) return [];
  return [
    {
      sectionKey: "commits",
      label: t("task:commits"),
      testId: "commits-section",
      commits: mergedCommits,
      defaultCollapsed: firstSection !== "commits",
      showActions: true,
    },
  ];
}

function createDivergedCommitSections(
  separated: ReturnType<typeof separateCommitHistories>,
  firstSection: ChangesSection,
  providerPRNumber: number | undefined,
  t: TFunction,
): CommitSection[] {
  const sections: CommitSection[] = [];
  if (separated.localCommits.length > 0) {
    sections.push({
      sectionKey: "local-checkout-commits",
      label: t("task:localCheckoutCommits"),
      testId: "local-checkout-commits-section",
      commits: separated.localCommits,
      defaultCollapsed: firstSection !== "commits",
      showActions: true,
    });
  }
  if (separated.providerCommits.length > 0) {
    sections.push({
      sectionKey: "current-pr-commits",
      label: t("task:prNumberVersion", { number: providerPRNumber ?? "" }),
      testId: "current-pr-commits-section",
      commits: separated.providerCommits,
      defaultCollapsed: true,
      showActions: false,
    });
  }
  return sections;
}

export function useHistoryExpansion(
  props: ChangesPanelTimelineContentProps,
  commitSections: CommitSection[],
): HistoryExpansionState {
  const hasLocalChanges = props.hasUnstaged || props.hasStaged;
  const firstSection = firstVisibleSection({
    hasPRFiles: props.hasPRFiles,
    hasUnstaged: props.hasUnstaged,
    hasStaged: props.hasStaged,
    showCommitsList: props.hasStaged || commitSections.length > 0,
    prFileCount: props.prFiles.length,
  });
  const defaultCollapsedByKey = useMemo(() => {
    const values = new Map<string, boolean>();
    if (props.hasPRFiles) values.set("pr-files", hasLocalChanges || firstSection !== "pr");
    for (const section of commitSections) values.set(section.sectionKey, section.defaultCollapsed);
    return values;
  }, [commitSections, firstSection, hasLocalChanges, props.hasPRFiles]);
  const [sectionOverrides, setSectionOverrides] = useState<Map<string, boolean>>(() => new Map());
  const [collapsedRepositories, setCollapsedRepositories] = useState<Set<string>>(() => new Set());
  const [collapsedInlineDirectories, setCollapsedInlineDirectories] = useState<Set<string>>(
    () => new Set(),
  );
  const [focusRequest, setFocusRequest] = useState<ChangesTimelineFocusRequest>();
  const previousComparisonToken = useRef<number | undefined>(undefined);

  useEffect(() => {
    expandForComparison(props.comparisonRequestToken, previousComparisonToken, commitSections, {
      setSectionOverrides,
      setFocusRequest,
    });
  }, [commitSections, props.comparisonRequestToken]);

  return useMemo(
    () => ({
      defaultCollapsedByKey,
      sectionOverrides,
      setSectionOverrides,
      collapsedRepositories,
      setCollapsedRepositories,
      collapsedInlineDirectories,
      setCollapsedInlineDirectories,
      focusRequest,
    }),
    [
      collapsedInlineDirectories,
      collapsedRepositories,
      defaultCollapsedByKey,
      focusRequest,
      sectionOverrides,
    ],
  );
}

function expandForComparison(
  comparisonRequestToken: number | undefined,
  previousTokenRef: MutableRefObject<number | undefined>,
  commitSections: CommitSection[],
  setters: {
    setSectionOverrides: Dispatch<SetStateAction<Map<string, boolean>>>;
    setFocusRequest: Dispatch<SetStateAction<ChangesTimelineFocusRequest | undefined>>;
  },
): void {
  if (comparisonRequestToken === undefined || comparisonRequestToken === previousTokenRef.current) {
    return;
  }
  previousTokenRef.current = comparisonRequestToken;
  setters.setSectionOverrides((current) => {
    const next = new Map(current);
    for (const section of commitSections) next.set(section.sectionKey, false);
    return next;
  });
  const sectionToFocus =
    commitSections.find((section) => section.sectionKey === "current-pr-commits") ??
    commitSections[0];
  if (sectionToFocus) {
    setters.setFocusRequest({
      rowKey: changesHistorySectionRowKey(sectionToFocus.sectionKey),
      token: comparisonRequestToken,
    });
  }
}

export function useHistorySections({
  props,
  commitSections,
  expansion,
  detailState,
  detailStateVersion,
  layout,
}: {
  props: ChangesPanelTimelineContentProps;
  commitSections: CommitSection[];
  expansion: HistoryExpansionState;
  detailState: ChangesInlineCommitState;
  detailStateVersion: number;
  layout: string;
}): ChangesHistorySectionInput[] {
  const { t } = useTranslation();
  return useMemo(
    () =>
      createHistorySections({
        props,
        commitSections,
        expansion,
        detailState,
        layout,
        t,
      }),
    [
      commitSections,
      detailState,
      detailStateVersion,
      expansion,
      layout,
      props.hasPRFiles,
      props.prFiles,
      t,
    ],
  );
}

function createHistorySections({
  props,
  commitSections,
  expansion,
  detailState,
  layout,
  t,
}: {
  props: ChangesPanelTimelineContentProps;
  commitSections: CommitSection[];
  expansion: HistoryExpansionState;
  detailState: ChangesInlineCommitState;
  layout: string;
  t: TFunction;
}): ChangesHistorySectionInput[] {
  const sections: ChangesHistorySectionInput[] = [];
  if (props.hasPRFiles) sections.push(createPRHistorySection(props, expansion, t));
  for (const section of commitSections) {
    sections.push(createCommitHistorySection(section, expansion, detailState, layout));
  }
  return sections;
}

function createPRHistorySection(
  props: ChangesPanelTimelineContentProps,
  expansion: HistoryExpansionState,
  t: TFunction,
): ChangesHistorySectionInput {
  return {
    kind: "pr",
    sectionKey: "pr-files",
    label: t("task:prChanges"),
    testId: "pr-files-section",
    collapsed:
      expansion.sectionOverrides.get("pr-files") ??
      expansion.defaultCollapsedByKey.get("pr-files") ??
      true,
    files: props.prFiles,
    collapsedRepositories: expansion.collapsedRepositories,
  };
}

function createCommitHistorySection(
  section: CommitSection,
  expansion: HistoryExpansionState,
  detailState: ChangesInlineCommitState,
  layout: string,
): ChangesHistorySectionInput {
  return {
    kind: "commits",
    sectionKey: section.sectionKey,
    label: section.label,
    testId: section.testId,
    collapsed:
      expansion.sectionOverrides.get(section.sectionKey) ??
      expansion.defaultCollapsedByKey.get(section.sectionKey) ??
      true,
    commits: section.commits.map((commit) => ({
      commit,
      detail: detailState.get(commit.detailTarget),
    })),
    collapsedRepositories: expansion.collapsedRepositories,
    collapsedDirectories: expansion.collapsedInlineDirectories,
    layout: layout === "tree" ? "tree" : "flat",
    showActions: section.showActions,
  };
}

export function useHistoryRows(
  historySections: ChangesHistorySectionInput[],
  hasLocalChanges: boolean,
): HistoryRows {
  const collapsedCommitKeys = useMemo(
    () => getCollapsedCommitKeys(historySections),
    [historySections],
  );
  return useMemo(
    () => buildHistoryRows(historySections, collapsedCommitKeys, hasLocalChanges),
    [collapsedCommitKeys, hasLocalChanges, historySections],
  );
}

function getCollapsedCommitKeys(historySections: ChangesHistorySectionInput[]): Set<string> {
  const keys = new Set<string>();
  for (const section of historySections) {
    if (section.kind !== "commits") continue;
    for (const item of section.commits) {
      if (!item.detail.expanded) {
        keys.add(changesCommitExpansionKey(section.sectionKey, item.commit.detailTarget));
      }
    }
  }
  return keys;
}

function buildHistoryRows(
  historySections: ChangesHistorySectionInput[],
  collapsedCommitKeys: Set<string>,
  hasLocalChanges: boolean,
): HistoryRows {
  const prSection = historySections.find((section) => section.kind === "pr");
  const beforeSections = prSection && !hasLocalChanges ? [prSection] : [];
  const afterSections = [
    ...(prSection && hasLocalChanges ? [prSection] : []),
    ...historySections.filter((section) => section.kind === "commits"),
  ];
  return {
    before: buildChangesHistoryTimelineRows(beforeSections, collapsedCommitKeys),
    after: buildChangesHistoryTimelineRows(afterSections, collapsedCommitKeys),
  };
}

export function useHistoryRowRenderer(
  props: ChangesPanelTimelineContentProps,
  commitSections: CommitSection[],
  detailState: ChangesInlineCommitState,
  expansion: HistoryExpansionState,
): (row: ChangesHistoryTimelineRow) => ReactNode {
  const sectionCommits = useMemo(() => indexSectionCommits(commitSections), [commitSections]);
  const actions = useMemo(() => createHistoryActions(props), [props]);

  return useCallback(
    (row: ChangesHistoryTimelineRow) => (
      <ChangesTimelineHistoryRow
        key={row.key}
        row={row}
        sectionCommits={sectionCommits.get(row.sectionKey)}
        actions={actions}
        onToggleSection={(sectionKey) => toggleHistorySection(sectionKey, expansion)}
        onToggleRepository={(sectionKey, repositoryName) => {
          const key = changesHistoryRepositoryExpansionKey(sectionKey, repositoryName);
          expansion.setCollapsedRepositories((current) => toggleSetValue(current, key));
        }}
        onToggleCommit={(_sectionKey, target, expanded) => {
          if (expanded) void detailState.expand(target);
          else detailState.collapse(target);
        }}
        onRetryCommit={(target) => void detailState.retry(target)}
        onToggleInlineDirectory={(sectionKey, target, path) => {
          const key = changesInlineDirectoryExpansionKey(
            sectionKey,
            commitDetailTargetKey(target),
            path,
          );
          expansion.setCollapsedInlineDirectories((current) => toggleSetValue(current, key));
        }}
      />
    ),
    [actions, detailState, expansion, sectionCommits],
  );
}

function indexSectionCommits(commitSections: CommitSection[]): Map<string, CommitItem[]> {
  const result = new Map<string, CommitItem[]>();
  for (const section of commitSections) result.set(section.sectionKey, section.commits);
  return result;
}

function createHistoryActions(props: ChangesPanelTimelineContentProps) {
  return {
    onOpenDiff: props.onOpenDiffFile,
    onOpenCommitDetail: props.onOpenCommitDetail,
    onRevertCommit: props.onRevertCommit,
    onAmendCommit: props.dialogs.handleOpenAmendDialog,
    onResetToCommit: props.dialogs.handleOpenResetDialog,
    onRepoPush: props.onRepoPush,
    onRepoCreatePR: props.onRepoCreatePR,
    repoDisplayName: props.repoDisplayName,
    perRepoStatus: props.perRepoStatus,
    prByRepo: props.prByRepo,
    pushDisabled: props.pushDisabled,
  };
}

function toggleHistorySection(sectionKey: string, expansion: HistoryExpansionState): void {
  const collapsed =
    expansion.sectionOverrides.get(sectionKey) ??
    expansion.defaultCollapsedByKey.get(sectionKey) ??
    true;
  expansion.setSectionOverrides((current) => new Map(current).set(sectionKey, !collapsed));
}

function toggleSetValue<T>(current: Set<T>, value: T): Set<T> {
  const next = new Set(current);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}
