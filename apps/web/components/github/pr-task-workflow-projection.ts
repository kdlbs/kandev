import type { useTranslation } from "react-i18next";
import type { PRCIState, TaskPR } from "@/lib/types/github";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { TaskPRInfo } from "./pr-task-automation";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";
import { aggregatePRCIState } from "./pr-ci-state";

export type PRTaskStatusSummaryData = ReturnType<typeof derivePRTaskStatusSummary>;
export type ProjectedPRTaskStatusSummary = Omit<PRTaskStatusSummaryData, "number"> & {
  number: number;
};

export type NegativeWorkflowApprovalDisclosure = {
  summaries: ProjectedPRTaskStatusSummary[];
  count: number;
  identity?: { number: number; repository?: string };
};

type DisclosureEntry = {
  pr?: TaskPR;
  repository?: string;
  summary: ProjectedPRTaskStatusSummary;
};

function compactPRLifecycleLabel(
  state: string,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  switch (state.toLowerCase()) {
    case "merged":
      return t("github:merged");
    case "closed":
      return t("github:closed");
    case "draft":
      return t("github:draft");
    case "open":
      return t("common:open");
    default:
      return null;
  }
}

function compactPRAggregateLabel(
  state: string | undefined,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  switch (state?.toLowerCase()) {
    case "failure":
      return t("github:needsAttention");
    case "pending":
      return t("common:pending");
    case "awaiting_review":
      return t("github:pendingReview");
    case "blocked":
      return t("github:blocked");
    case "ready":
      return null;
    case "queued":
      return t("github:mergeQueueStateQueued");
    case "passing":
      return t("github:checksPassed");
    case "draft":
      return t("github:draft");
    case "merged":
      return t("github:merged");
    case "closed":
      return t("github:closed");
    default:
      return null;
  }
}

function compactCIAccessibleLabel(
  state: TaskPRInfo["ciState"],
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  switch (state) {
    case "failed":
      return t("github:checksFailed");
    case "approval_required":
      return t("github:workflowAwaitingApproval");
    case "attention":
      return t("github:workflowNeedsAttention");
    case "in_progress":
      return t("github:checksRunning");
    default:
      return null;
  }
}

export function getCompactPRStatusAccessibleLabels(
  prInfo: TaskPRInfo,
  t: ReturnType<typeof useTranslation>["t"],
): string[] {
  return [
    compactPRLifecycleLabel(prInfo.state, t),
    compactPRAggregateLabel(prInfo.aggregateState, t),
    prInfo.workflowApprovalRequired ? t("github:workflowAwaitingApproval") : null,
    compactCIAccessibleLabel(prInfo.ciState, t),
  ]
    .filter((label): label is string => label !== null)
    .filter((label, index, labels) => labels.indexOf(label) === index);
}

function compactSummaryIsNewerThanFullPRs(prs: TaskPR[], prInfo: TaskPRInfo): boolean {
  const summaryUpdatedAt = parseStrictRfc3339Timestamp(prInfo.statusSummaryUpdatedAt);
  if (summaryUpdatedAt === null) return false;
  const fullPRFreshness = prs.map((pr) => {
    const syncedAt = parseStrictRfc3339Timestamp(pr.last_synced_at ?? undefined);
    if (syncedAt === null) return null;
    const attentionAt = parseStrictRfc3339Timestamp(pr.workflow_attention?.observed_at);
    return attentionAt !== null && attentionAt > syncedAt ? attentionAt : syncedAt;
  });
  return fullPRFreshness.every((updatedAt) => updatedAt !== null && summaryUpdatedAt > updatedAt);
}

export function compactWorkflowApprovalIsNewerThanFullPRs(
  prs: TaskPR[],
  prInfo?: TaskPRInfo,
): boolean {
  if (!prInfo || typeof prInfo.workflowApprovalRequired !== "boolean" || prs.length === 0) {
    return false;
  }
  return compactSummaryIsNewerThanFullPRs(prs, prInfo);
}

export function compactCIIsNewerThanFullPRs(prs: TaskPR[], prInfo?: TaskPRInfo): boolean {
  if (!prInfo || typeof prInfo.ciState !== "string" || prs.length === 0) {
    return false;
  }
  return compactSummaryIsNewerThanFullPRs(prs, prInfo);
}

/** A compact CI field is authoritative when no full record can be newer. */
export function isCompactCIAuthoritative(prs: TaskPR[], prInfo?: TaskPRInfo): boolean {
  if (!prInfo || typeof prInfo.ciState !== "string") return false;
  if (prs.length === 0) return true;
  return compactCIIsNewerThanFullPRs(prs, prInfo);
}

export function getCompactWorkflowStatusSummaries(
  prInfo: TaskPRInfo,
  includeCI = false,
): ProjectedPRTaskStatusSummary[] {
  const summaries = new Map<string, ProjectedPRTaskStatusSummary>();
  const addRow = (
    number: number,
    repository: string | undefined,
    row: PRTaskStatusSummaryData["rows"][number],
  ) => {
    const key = `${repository ?? ""}#${number}`;
    const summary = summaries.get(key) ?? { number, title: "", rows: [] };
    const detail = repository
      ? {
          key: "github:prTaskStatusRepositoryNumber",
          values: { repository, number },
        }
      : undefined;
    summary.rows.push({ ...row, ...(detail ? { detail } : {}) });
    summaries.set(key, summary);
  };

  if (prInfo.workflowApprovalRequired === true) {
    addRow(prInfo.workflowApprovalPRNumber ?? prInfo.number, prInfo.workflowApprovalRepository, {
      kind: "ci",
      id: WORKFLOW_ATTENTION_ROW_ID,
      status: "awaiting_approval",
      tone: "warning",
    });
  }
  if (includeCI) {
    const ciRow = compactCIRow(prInfo);
    if (ciRow) {
      addRow(prInfo.ciPRNumber ?? prInfo.number, prInfo.ciRepository, ciRow);
    }
  }
  if (prInfo.hasMergeConflicts === true) {
    addRow(prInfo.mergeConflictPRNumber ?? prInfo.number, prInfo.mergeConflictRepository, {
      kind: "merge",
      id: "merge-conflict",
      status: "conflicts",
      tone: "danger",
    });
  }
  return [...summaries.values()];
}

type ProjectedSummaryRow = ProjectedPRTaskStatusSummary["rows"][number];

const CHECK_OUTCOME_STATUSES = new Set<ProjectedSummaryRow["status"]>([
  "passed",
  "failed",
  "in_progress",
  "checks_not_successful",
  "all_skipped",
]);

const WORKFLOW_ATTENTION_STATUSES = new Set<ProjectedSummaryRow["status"]>([
  "awaiting_approval",
  "workflow_attention",
  "workflow_unavailable",
]);

const WORKFLOW_ATTENTION_ROW_ID = "workflow-attention";

type ProjectedDisclosureEntry = PRTaskStatusSummaryData & { repository?: string };

function isCheckOutcomeRow(row: ProjectedSummaryRow): boolean {
  return row.kind === "ci" && CHECK_OUTCOME_STATUSES.has(row.status);
}

function supersededWorkflowRow(row: ProjectedSummaryRow, ciState: TaskPRInfo["ciState"]): boolean {
  if (row.kind !== "ci") return false;
  if (ciState === "approval_required") return WORKFLOW_ATTENTION_STATUSES.has(row.status);
  if (ciState === "attention") return row.status === "workflow_unavailable";
  return false;
}

function compactCIRow(prInfo: TaskPRInfo): ProjectedSummaryRow | null {
  switch (prInfo.ciState) {
    case "failed":
      return { kind: "ci", id: "ci-state", status: "failed", tone: "danger" };
    case "approval_required":
      return {
        kind: "ci",
        id: WORKFLOW_ATTENTION_ROW_ID,
        status: "awaiting_approval",
        tone: "warning",
      };
    case "attention":
      return { kind: "ci", id: "ci-state", status: "checks_not_successful", tone: "warning" };
    case "in_progress":
      return { kind: "ci", id: "ci-state", status: "in_progress", tone: "warning" };
    default:
      return null;
  }
}

function stripDisclosureRepositories(
  entries: ProjectedDisclosureEntry[],
): PRTaskStatusSummaryData[] {
  return entries.map(({ repository: _repository, ...summary }) => summary);
}

function findProjectedSummary(
  entries: ProjectedDisclosureEntry[],
  number: number,
  repository: string | undefined,
): ProjectedDisclosureEntry | undefined {
  const numbered = entries.filter((entry) => Number(entry.number) === number);
  if (repository) {
    return numbered.find((entry) => normalizeRepositoryName(entry.repository) === repository);
  }
  return numbered.length === 1 ? numbered[0] : undefined;
}

/**
 * Apply a newer compact CI observation to already-derived full disclosure
 * rows. Check outcomes are mutually exclusive, so a newer aggregate replaces
 * every stale outcome for its selected PR. Independent workflow reasons stay
 * visible, and a newer approval replaces older workflow-attention rows.
 */
export function applyCompactCIDisclosure(
  summaries: PRTaskStatusSummaryData[],
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  ciAuthoritative: boolean,
): PRTaskStatusSummaryData[] {
  if (!ciAuthoritative || !prInfo || typeof prInfo.ciState !== "string") return summaries;
  const entries: ProjectedDisclosureEntry[] = summaries.map((summary, index) => ({
    ...summary,
    rows: [...summary.rows],
    repository: prs[index] ? taskPRRepository(prs[index]) : undefined,
  }));
  if (prInfo.ciState === "none") {
    for (const entry of entries) {
      entry.rows = entry.rows.filter((row) => !isCheckOutcomeRow(row));
    }
    return stripDisclosureRepositories(entries);
  }
  const row = compactCIRow(prInfo);
  if (!row) return stripDisclosureRepositories(entries);
  const selectedNumber = prInfo.ciPRNumber ?? prInfo.number;
  const selectedRepository = normalizeRepositoryName(prInfo.ciRepository);
  const match = findProjectedSummary(entries, selectedNumber, selectedRepository);
  const target: ProjectedDisclosureEntry =
    match ??
    ({
      number: selectedNumber,
      title: "",
      rows: [],
      repository: prInfo.ciRepository,
    } satisfies ProjectedDisclosureEntry);
  if (!match) entries.push(target);
  target.rows = target.rows.filter(
    (existing) => !isCheckOutcomeRow(existing) && !supersededWorkflowRow(existing, prInfo.ciState),
  );
  if (
    prInfo.ciState === "attention" &&
    target.rows.some((existing) => existing.id === WORKFLOW_ATTENTION_ROW_ID)
  ) {
    return stripDisclosureRepositories(entries);
  }
  const rowWithIdentity = prInfo.ciRepository
    ? {
        ...row,
        detail: {
          key: "github:prTaskStatusRepositoryNumber",
          values: { repository: prInfo.ciRepository, number: selectedNumber },
        },
      }
    : row;
  target.rows.push(rowWithIdentity);
  return stripDisclosureRepositories(entries);
}

function taskPRRepository(pr: TaskPR): string | undefined {
  const owner = pr.owner.trim();
  const repository = pr.repo.trim();
  return owner && repository ? `${owner}/${repository}` : undefined;
}

function normalizeRepositoryName(repository: string | undefined): string | undefined {
  const normalized = repository?.trim().toLowerCase();
  return normalized || undefined;
}

function findDisclosureEntry(
  entries: DisclosureEntry[],
  number: number,
  repository: string | undefined,
): DisclosureEntry | undefined {
  const numberedEntries = entries.filter((entry) => entry.summary.number === number);
  const normalizedRepository = normalizeRepositoryName(repository);
  if (normalizedRepository) {
    return numberedEntries.find(
      (entry) => normalizeRepositoryName(entry.repository) === normalizedRepository,
    );
  }
  return numberedEntries.length === 1 ? numberedEntries[0] : undefined;
}

function getConflictProjectionRepository(prInfo: TaskPRInfo, number: number): string | undefined {
  return (prInfo.mergeConflictPRNumber ?? prInfo.number) === number
    ? prInfo.mergeConflictRepository
    : undefined;
}

function getTerminalProjectionRepository(prInfo: TaskPRInfo): string | undefined {
  const repositories = [
    (prInfo.workflowApprovalPRNumber ?? prInfo.number) === prInfo.number
      ? prInfo.workflowApprovalRepository
      : undefined,
    (prInfo.mergeConflictPRNumber ?? prInfo.number) === prInfo.number
      ? prInfo.mergeConflictRepository
      : undefined,
  ]
    .map(normalizeRepositoryName)
    .filter((repository): repository is string => repository !== undefined);
  const distinctRepositories = [...new Set(repositories)];
  return distinctRepositories.length === 1 ? distinctRepositories[0] : undefined;
}

function compactTerminalStateRow(state: string): PRTaskStatusSummaryData["rows"][number] | null {
  switch (state.trim().toLowerCase()) {
    case "merged":
      return { kind: "state", status: "merged", tone: "merged" };
    case "closed":
      return { kind: "state", status: "closed", tone: "danger" };
    default:
      return null;
  }
}

function isSupersededByCompactConflict(row: PRTaskStatusSummaryData["rows"][number]): boolean {
  return (
    row.kind === "merge" &&
    (row.status === "ready" || row.status === "mergeable" || row.status === "conflicts")
  );
}

function cachedPRSummary(
  pr: TaskPR,
  summary?: PRTaskStatusSummaryData,
): ProjectedPRTaskStatusSummary {
  if (summary) return { ...summary, number: pr.pr_number };
  const author = pr.author_login.trim();
  return {
    number: pr.pr_number,
    title: pr.pr_title,
    ...(author ? { author } : {}),
    rows: [],
  };
}

/** Keep full PR identity while applying the newer negative approval and conflict projection. */
export function getNegativeWorkflowApprovalDisclosure(
  prs: TaskPR[],
  fullSummaries: PRTaskStatusSummaryData[],
  prInfo: TaskPRInfo,
  compactSummaries = getCompactWorkflowStatusSummaries(prInfo),
): NegativeWorkflowApprovalDisclosure {
  const entries: DisclosureEntry[] = prs.map((pr, index) => {
    const summary = cachedPRSummary(pr, fullSummaries[index]);
    return {
      pr,
      repository: taskPRRepository(pr),
      summary: {
        ...summary,
        rows: summary.rows.filter(
          (row) => !(row.id === WORKFLOW_ATTENTION_ROW_ID && row.status === "awaiting_approval"),
        ),
      },
    };
  });

  const terminalState = compactTerminalStateRow(prInfo.state);

  for (const compactSummary of compactSummaries) {
    const conflictRows = compactSummary.rows.filter((row) => row.id === "merge-conflict");
    if (conflictRows.length === 0) continue;

    const repository =
      getConflictProjectionRepository(prInfo, compactSummary.number) ??
      getProjectedPRRepository(prInfo, compactSummary.number);
    const matchingEntry = findDisclosureEntry(entries, compactSummary.number, repository);

    if (matchingEntry) {
      matchingEntry.summary = {
        ...matchingEntry.summary,
        rows: [
          ...matchingEntry.summary.rows.filter((row) => !isSupersededByCompactConflict(row)),
          ...conflictRows,
        ],
      };
      continue;
    }

    entries.push({
      repository,
      summary: { ...compactSummary, rows: conflictRows },
    });
  }

  if (prInfo.hasMergeConflicts === false) {
    const conflictNumber = prInfo.mergeConflictPRNumber ?? prInfo.number;
    const conflictEntry = findDisclosureEntry(
      entries,
      conflictNumber,
      getConflictProjectionRepository(prInfo, conflictNumber),
    );
    if (conflictEntry) {
      conflictEntry.summary = {
        ...conflictEntry.summary,
        rows: conflictEntry.summary.rows.filter(
          (row) => !(row.kind === "merge" && row.status === "conflicts"),
        ),
      };
    }
  }

  const terminalEntry = terminalState
    ? findDisclosureEntry(entries, prInfo.number, getTerminalProjectionRepository(prInfo))
    : undefined;
  if (terminalState && terminalEntry) {
    terminalEntry.summary = {
      ...terminalEntry.summary,
      rows: [
        terminalState,
        ...terminalEntry.summary.rows.filter((row) => row.kind !== "state" && row.kind !== "merge"),
      ],
    };
  }

  const summaries = entries.map(({ summary }) => summary);
  const onlyEntry = entries.length === 1 ? entries[0] : undefined;
  return {
    summaries,
    count: summaries.length,
    ...(onlyEntry
      ? {
          identity: {
            number: onlyEntry.summary.number,
            ...(onlyEntry.repository ? { repository: onlyEntry.repository } : {}),
          },
        }
      : {}),
  };
}

export function getCompactStaleWorkflowPRs(prInfo: TaskPRInfo) {
  if (prInfo.workflowApprovalRequired !== true || !prInfo.workflowApprovalStale) return [];
  return [
    {
      number: prInfo.workflowApprovalPRNumber ?? prInfo.number,
      repository: prInfo.workflowApprovalRepository,
    },
  ];
}

export function getProjectedPRRepository(prInfo: TaskPRInfo | undefined, number: number) {
  if (!prInfo) return undefined;
  if (
    prInfo.workflowApprovalRequired === true &&
    (prInfo.workflowApprovalPRNumber ?? prInfo.number) === number
  ) {
    return prInfo.workflowApprovalRepository;
  }
  if (
    prInfo.hasMergeConflicts === true &&
    (prInfo.mergeConflictPRNumber ?? prInfo.number) === number
  ) {
    return prInfo.mergeConflictRepository;
  }
  return undefined;
}

export function getTaskPRIconCIState(
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  compactWorkflowApprovalAuthoritative: boolean,
): PRCIState {
  const fullState = prs.length > 0 ? aggregatePRCIState(prs).state : "none";
  if (!prInfo) return fullState;
  if (prs.length === 0) {
    if (prInfo.ciState) return prInfo.ciState;
    return prInfo.workflowApprovalRequired === true ? "approval_required" : "none";
  }
  if (compactCIIsNewerThanFullPRs(prs, prInfo) && prInfo.ciState) return prInfo.ciState;
  if (!compactWorkflowApprovalAuthoritative) return fullState;
  if (prInfo.workflowApprovalRequired === true) {
    return prInfo.ciState && prInfo.ciState !== "none" ? prInfo.ciState : "approval_required";
  }
  if (prInfo.workflowApprovalRequired === false && fullState === "approval_required") return "none";
  return fullState;
}

export function getTaskPRIconDisclosureProjection(
  prs: TaskPR[],
  fullSummaries: PRTaskStatusSummaryData[],
  prInfo: TaskPRInfo | undefined,
  compactWorkflowApprovalAuthoritative: boolean,
  compactCIAuthoritative: boolean,
) {
  if ((!compactWorkflowApprovalAuthoritative && !compactCIAuthoritative) || !prInfo) {
    return { summaries: fullSummaries };
  }

  if (compactWorkflowApprovalAuthoritative) {
    const compactSummaries = getCompactWorkflowStatusSummaries(prInfo, compactCIAuthoritative);
    if (prInfo.workflowApprovalRequired === false) {
      const negativeDisclosure = getNegativeWorkflowApprovalDisclosure(
        prs,
        fullSummaries,
        prInfo,
        compactSummaries,
      );
      const summaries = applyCompactCIDisclosure(
        negativeDisclosure.summaries,
        prs,
        prInfo,
        compactCIAuthoritative,
      );
      return {
        summaries,
        count: summaries.length,
        identity: projectedDisclosureIdentity(prInfo, summaries),
      };
    }
    return {
      summaries: compactSummaries,
      count: compactSummaries.length,
      identity: projectedDisclosureIdentity(prInfo, compactSummaries),
    };
  }

  const summaries = applyCompactCIDisclosure(fullSummaries, prs, prInfo, compactCIAuthoritative);
  return {
    summaries,
    count: summaries.length,
    identity: projectedDisclosureIdentity(prInfo, summaries),
  };
}

function projectedDisclosureIdentity(prInfo: TaskPRInfo, summaries: PRTaskStatusSummaryData[]) {
  if (summaries.length !== 1) return undefined;
  const number = Number(summaries[0].number);
  const repository =
    (prInfo.ciPRNumber ?? prInfo.number) === number
      ? prInfo.ciRepository
      : getProjectedPRRepository(prInfo, number);
  return { number, repository };
}
