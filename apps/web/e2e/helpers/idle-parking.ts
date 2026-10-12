import { DatabaseSync } from "./node-sqlite";

export function readIdleSuspensionState(databasePath: string, sessionId: string): string | null {
  const database = new DatabaseSync(databasePath);
  try {
    const row = database
      .prepare("SELECT idle_suspension_state FROM executors_running WHERE session_id = ?")
      .get(sessionId) as { idle_suspension_state?: string } | undefined;
    return row?.idle_suspension_state ?? null;
  } finally {
    database.close();
  }
}

export function readIdleSuspensionSnapshot(databasePath: string, sessionId: string) {
  const database = new DatabaseSync(databasePath);
  try {
    return database
      .prepare(
        `SELECT er.idle_suspension_state AS idleSuspensionState, er.status AS executorStatus,
                er.resumable AS resumable, LENGTH(COALESCE(er.resume_token, '')) > 0 AS hasResumeToken,
                ts.state AS sessionState, ts.updated_at AS sessionUpdatedAt,
                w.acp_idle_suspension_enabled AS policyEnabled,
                w.acp_idle_timeout_minutes AS policyTimeoutMinutes,
                w.updated_at AS policyUpdatedAt
           FROM executors_running er
           JOIN task_sessions ts ON ts.id = er.session_id
           JOIN tasks t ON t.id = er.task_id
           JOIN workspaces w ON w.id = t.workspace_id
          WHERE er.session_id = ?`,
      )
      .get(sessionId) as
      | {
          idleSuspensionState: string;
          executorStatus: string;
          resumable: number;
          hasResumeToken: number;
          sessionState: string;
          sessionUpdatedAt: string;
          policyEnabled: number;
          policyTimeoutMinutes: number;
          policyUpdatedAt: string;
        }
      | undefined;
  } finally {
    database.close();
  }
}

export function readSessionMessageCount(
  databasePath: string,
  sessionId: string,
  authorType?: string,
): number {
  const database = new DatabaseSync(databasePath);
  try {
    const row = authorType
      ? (database
          .prepare(
            "SELECT COUNT(*) AS count FROM task_session_messages WHERE task_session_id = ? AND author_type = ?",
          )
          .get(sessionId, authorType) as { count: number })
      : (database
          .prepare("SELECT COUNT(*) AS count FROM task_session_messages WHERE task_session_id = ?")
          .get(sessionId) as { count: number });
    return Number(row.count);
  } finally {
    database.close();
  }
}

export function readSessionContentMessageCount(
  databasePath: string,
  sessionId: string,
  authorType: string,
): number {
  const database = new DatabaseSync(databasePath);
  try {
    const row = database
      .prepare(
        "SELECT COUNT(*) AS count FROM task_session_messages WHERE task_session_id = ? AND author_type = ? AND type = 'message'",
      )
      .get(sessionId, authorType) as { count: number };
    return Number(row.count);
  } finally {
    database.close();
  }
}

export async function updateWorkspaceIdlePolicy(
  baseUrl: string,
  workspaceId: string,
  enabled: boolean,
  timeoutMinutes: number,
): Promise<void> {
  const response = await fetch(`${baseUrl}/api/v1/workspaces/${workspaceId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      acp_idle_suspension_enabled: enabled,
      acp_idle_timeout_minutes: timeoutMinutes,
    }),
  });
  if (!response.ok) {
    throw new Error(
      `workspace policy update failed with ${response.status}: ${await response.text()}`,
    );
  }
}
