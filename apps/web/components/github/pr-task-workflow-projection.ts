import type { useTranslation } from "react-i18next";
import type { TaskPR } from "@/lib/types/github";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { TaskPRInfo } from "./pr-task-automation";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";

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

export function getCompactPRStatusAccessibleLabels(
  prInfo: TaskPRInfo,
  t: ReturnType<typeof useTranslation>["t"],
): string[] {
  return [
    compactPRLifecycleLabel(prInfo.state, t),
    compactPRAggregateLabel(prInfo.aggregateState, t),
    prInfo.workflowApprovalRequired ? t("github:workflowAwaitingApproval") : null,
  ]
    .filter((label): label is string => label !== null)
    .filter((label, index, labels) => labels.indexOf(label) === index);
}

export function compactWorkflowApprovalIsNewerThanFullPRs(
  prs: TaskPR[],
  prInfo?: TaskPRInfo,
): boolean {
  if (!prInfo || typeof prInfo.workflowApprovalRequired !== "boolean" || prs.length === 0) {
    return false;
  }
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

export function getCompactWorkflowStatusSummaries(
  prInfo: TaskPRInfo,
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
      id: "workflow-attention",
      status: "awaiting_approval",
      tone: "warning",
    });
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
          (row) => !(row.id === "workflow-attention" && row.status === "awaiting_approval"),
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
