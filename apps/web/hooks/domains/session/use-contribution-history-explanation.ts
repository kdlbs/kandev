"use client";

import { useEffect, useMemo, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { isFullCommitSHA } from "./remote-contribution-relation";

export type ContributionHistoryExplanationKind = "local_rebase" | "unexplained";

export type ContributionHistoryExplanationReason =
  | "matched_reflog"
  | "no_matching_reflog"
  | "missing_objects"
  | "ambiguous"
  | "stale"
  | "limit"
  | "unavailable";

export type ContributionHistoryExplanation = {
  repo?: string;
  branch: string;
  expected_local_head: string;
  expected_remote_head: string;
  kind: ContributionHistoryExplanationKind;
  reason: ContributionHistoryExplanationReason;
  onto_head?: string;
  task_commit_count?: number;
  published_commit_count?: number;
  new_base_commit_count?: number;
};

export type ContributionHistoryExplanationTarget = {
  sessionId: string;
  workspaceId: string;
  repositoryScope: string;
  branch: string;
  selectedPRKey: string;
  expectedLocalHead: string;
  expectedRemoteHead: string;
};

export type ContributionHistoryExplanationStatus = "idle" | "loading" | "ready" | "unavailable";

type ExplanationSnapshot = {
  status: ContributionHistoryExplanationStatus;
  explanation: ContributionHistoryExplanation | null;
  error: unknown;
};

type ExplanationEntry = ExplanationSnapshot & {
  listeners: Set<() => void>;
};

type BoundExplanationSnapshot = {
  key: string | null;
  snapshot: ExplanationSnapshot;
};

const ACTION = "worktree.contribution_history_explanation";
const REQUEST_TIMEOUT_MS = 2_500;
const entries = new Map<string, ExplanationEntry>();

function emptySnapshot(): ExplanationSnapshot {
  return { status: "idle", explanation: null, error: null };
}

function notify(entry: ExplanationEntry) {
  for (const listener of entry.listeners) listener();
}

function targetIsRequestable(
  target: ContributionHistoryExplanationTarget | null,
): target is ContributionHistoryExplanationTarget {
  return Boolean(
    target?.sessionId &&
    target.workspaceId &&
    target.repositoryScope !== undefined &&
    target.branch &&
    target.selectedPRKey &&
    isFullCommitSHA(target.expectedLocalHead) &&
    isFullCommitSHA(target.expectedRemoteHead),
  );
}

export function contributionHistoryExplanationKey(
  target: ContributionHistoryExplanationTarget | null,
): string | null {
  if (!targetIsRequestable(target)) return null;
  return JSON.stringify([
    target.workspaceId,
    target.sessionId,
    target.repositoryScope,
    target.branch,
    target.selectedPRKey,
    target.expectedLocalHead,
    target.expectedRemoteHead,
  ]);
}

function responseMatchesTarget(
  response: ContributionHistoryExplanation,
  target: ContributionHistoryExplanationTarget,
): boolean {
  return (
    (response.repo ?? "") === target.repositoryScope &&
    response.branch === target.branch &&
    response.expected_local_head === target.expectedLocalHead &&
    response.expected_remote_head === target.expectedRemoteHead
  );
}

function startEntry(key: string, target: ContributionHistoryExplanationTarget): ExplanationEntry {
  const entry: ExplanationEntry = {
    ...emptySnapshot(),
    status: "loading",
    listeners: new Set(),
  };
  entries.set(key, entry);

  const client = getWebSocketClient();
  if (!client) {
    entry.status = "unavailable";
    return entry;
  }

  try {
    const request = client.request<ContributionHistoryExplanation>(
      ACTION,
      {
        session_id: target.sessionId,
        repo: target.repositoryScope,
        branch: target.branch,
        expected_local_head: target.expectedLocalHead,
        expected_remote_head: target.expectedRemoteHead,
      },
      REQUEST_TIMEOUT_MS,
    );
    void request.then(
      (response) => {
        if (entries.get(key) !== entry) return;
        if (!responseMatchesTarget(response, target)) {
          entry.status = "unavailable";
          entry.error = new Error("contribution history explanation identity changed");
        } else {
          entry.status = "ready";
          entry.explanation = response;
        }
        notify(entry);
      },
      (error: unknown) => {
        if (entries.get(key) !== entry) return;
        entry.status = "unavailable";
        entry.error = error;
        notify(entry);
      },
    );
  } catch (error) {
    entry.status = "unavailable";
    entry.error = error;
  }
  return entry;
}

function getOrStartEntry(
  key: string,
  target: ContributionHistoryExplanationTarget,
): ExplanationEntry {
  return entries.get(key) ?? startEntry(key, target);
}

function snapshotOf(entry: ExplanationEntry): ExplanationSnapshot {
  return {
    status: entry.status,
    explanation: entry.explanation,
    error: entry.error,
  };
}

export function useContributionHistoryExplanation(
  target: ContributionHistoryExplanationTarget | null,
  enabled: boolean,
) {
  const requestTarget = useMemo(
    () => ({
      sessionId: target?.sessionId ?? "",
      workspaceId: target?.workspaceId ?? "",
      repositoryScope: target?.repositoryScope ?? "",
      branch: target?.branch ?? "",
      selectedPRKey: target?.selectedPRKey ?? "",
      expectedLocalHead: target?.expectedLocalHead ?? "",
      expectedRemoteHead: target?.expectedRemoteHead ?? "",
    }),
    [
      target?.sessionId,
      target?.workspaceId,
      target?.repositoryScope,
      target?.branch,
      target?.selectedPRKey,
      target?.expectedLocalHead,
      target?.expectedRemoteHead,
    ],
  );
  const key = enabled ? contributionHistoryExplanationKey(requestTarget) : null;
  const [boundSnapshot, setBoundSnapshot] = useState<BoundExplanationSnapshot>(() => ({
    key: null,
    snapshot: emptySnapshot(),
  }));
  const snapshot = boundSnapshot.key === key ? boundSnapshot.snapshot : emptySnapshot();

  useEffect(() => {
    if (!key || !targetIsRequestable(requestTarget)) {
      setBoundSnapshot({ key: null, snapshot: emptySnapshot() });
      return;
    }

    const entry = getOrStartEntry(key, requestTarget);
    let active = true;
    const sync = () => {
      if (active) setBoundSnapshot({ key, snapshot: snapshotOf(entry) });
    };
    entry.listeners.add(sync);
    sync();

    return () => {
      active = false;
      entry.listeners.delete(sync);
      if (entry.listeners.size === 0) entries.delete(key);
    };
  }, [key, requestTarget]);

  return {
    ...snapshot,
    isLoading: snapshot.status === "loading",
    cancel: () => undefined,
  };
}

/** Test-only registry cleanup. Mounted consumers keep production entries alive. */
export function resetContributionHistoryExplanationForTests() {
  entries.clear();
}
