import path from "node:path";
import { DatabaseSync } from "./node-sqlite";

export type ManagedRuntimeBackend = { tmpDir: string };

export function readManagedRuntimeNativeID(
  backend: ManagedRuntimeBackend,
  sessionId: string,
): string {
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    database.exec("PRAGMA busy_timeout=5000");
    const row = database
      .prepare("SELECT downstream_acp_session_id FROM task_sessions WHERE id = ?")
      .get(sessionId) as { downstream_acp_session_id?: string } | undefined;
    return row?.downstream_acp_session_id ?? "";
  } finally {
    database.close();
  }
}

export function readManagedRuntimeResumeToken(
  backend: ManagedRuntimeBackend,
  sessionId: string,
): string {
  const database = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  try {
    database.exec("PRAGMA busy_timeout=5000");
    const row = database
      .prepare("SELECT resume_token FROM executors_running WHERE session_id = ?")
      .get(sessionId) as { resume_token?: string } | undefined;
    return row?.resume_token ?? "";
  } finally {
    database.close();
  }
}
