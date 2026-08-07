import { useEffect, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";

export type SessionDeleteWarning = {
  uncommittedFiles: number;
  unpushedCommits: number;
};

/**
 * warning is null while nothing to warn about is known yet (or ever was);
 * isLoaded distinguishes "still fetching" from "resolved" so a caller can
 * gate an irreversible confirm control on it — the spec requires the counts
 * be stated before the confirm control is used
 * (docs/specs/session-delete-resource-cleanup: "Confirmation surface"), which
 * a UI that never blocks the control on the fetch resolving cannot guarantee
 * under real network latency. isLoaded is true whenever there's nothing left
 * to wait for: no dialog open, no session known, or the fetch settled
 * (success, failure, or no WS client available) for the current session.
 */
export type SessionDeleteWarningState = {
  warning: SessionDeleteWarning | null;
  isLoaded: boolean;
};

type SessionGitSnapshotSummary = {
  branch?: string;
  ahead?: number;
  files?: Record<string, unknown>;
};

// `session.git.snapshots` returns a per-session history ordered newest-first
// (not one row per repo), so a multi-repo session's snapshots for different
// branches are interleaved with older rows for the same branch. A modest
// window is enough to find the latest row per branch in practice without
// pulling a session's entire history for a confirmation dialog.
const SNAPSHOT_FETCH_LIMIT = 20;

// useSessionDeleteWarning fetches the session's git snapshot history when a
// delete confirmation dialog opens, so the dialog can warn about uncommitted
// files and unpushed commits before the user confirms
// (docs/specs/session-delete-resource-cleanup). It dedupes to the latest
// snapshot per branch (see SNAPSHOT_FETCH_LIMIT) before summing counts, so a
// multi-repo session's branches are each counted once.
export function useSessionDeleteWarning(
  open: boolean,
  sessionId: string | null | undefined,
): SessionDeleteWarningState {
  const [result, setResult] = useState<{ sessionId: string; warning: SessionDeleteWarning } | null>(
    null,
  );
  const [loadedSessionId, setLoadedSessionId] = useState<string | null>(null);

  useEffect(() => {
    if (!open || !sessionId) {
      setResult(null);
      setLoadedSessionId(null);
      return;
    }
    const client = getWebSocketClient();
    if (!client) {
      // No WS client to fetch from — nothing further will resolve this
      // session, so treat it as settled rather than blocking the caller's
      // confirm control forever.
      setLoadedSessionId(sessionId);
      return;
    }
    let cancelled = false;
    client
      .request<{ snapshots?: SessionGitSnapshotSummary[] }>("session.git.snapshots", {
        session_id: sessionId,
        limit: SNAPSHOT_FETCH_LIMIT,
      })
      .then((response) => {
        if (cancelled) return;
        setResult({ sessionId, warning: summarizeSnapshots(response?.snapshots ?? []) });
      })
      .catch(() => {
        // Fetch failure is not user-actionable here; the dialog just omits
        // the warning block rather than blocking the delete flow forever.
      })
      .finally(() => {
        if (cancelled) return;
        setLoadedSessionId(sessionId);
      });
    return () => {
      cancelled = true;
    };
  }, [open, sessionId]);

  const warning = result && result.sessionId === sessionId ? result.warning : null;
  const isLoaded = !open || !sessionId || loadedSessionId === sessionId;
  return { warning, isLoaded };
}

function summarizeSnapshots(snapshots: SessionGitSnapshotSummary[]): SessionDeleteWarning {
  const seenBranches = new Set<string>();
  let uncommittedFiles = 0;
  let unpushedCommits = 0;
  for (const snapshot of snapshots) {
    const branchKey = snapshot.branch ?? "";
    if (seenBranches.has(branchKey)) continue;
    seenBranches.add(branchKey);
    uncommittedFiles += Object.keys(snapshot.files ?? {}).length;
    unpushedCommits += snapshot.ahead ?? 0;
  }
  return { uncommittedFiles, unpushedCommits };
}
