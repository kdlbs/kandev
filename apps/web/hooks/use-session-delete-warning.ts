import { useEffect, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";

export type SessionDeleteWarning = {
  uncommittedFiles: number;
  unpushedCommits: number;
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
//
// Returns null while the dialog is closed, no session is known, or the fetch
// hasn't resolved (or failed) yet — callers treat null the same as "nothing
// to warn about" and simply omit the warning block, matching the "clean,
// level with remote" scenario.
export function useSessionDeleteWarning(
  open: boolean,
  sessionId: string | null | undefined,
): SessionDeleteWarning | null {
  const [result, setResult] = useState<{ sessionId: string; warning: SessionDeleteWarning } | null>(
    null,
  );

  useEffect(() => {
    if (!open || !sessionId) {
      setResult(null);
      return;
    }
    const client = getWebSocketClient();
    if (!client) return;
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
        // the warning block rather than blocking the delete flow.
      });
    return () => {
      cancelled = true;
    };
  }, [open, sessionId]);

  return result && result.sessionId === sessionId ? result.warning : null;
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
