import { randomUUID } from "node:crypto";
import { DatabaseSync } from "../../helpers/node-sqlite";

export function seedKubernetesTaskEnvironment(
  database: string,
  taskId: string,
  sessionId: string,
  executorId: string,
  profileId: string,
) {
  const values = [taskId, sessionId, executorId, profileId];
  if (values.some((value) => !/^[a-zA-Z0-9-]+$/.test(value))) {
    throw new Error("test fixture identities must be alphanumeric UUID-like values");
  }
  const environmentId = `environment-${randomUUID()}`;
  const sqlite = new DatabaseSync(database);
  let transactionOpen = false;
  try {
    // The backend may still be committing the session or environment row when
    // this fixture attaches its synthetic Kubernetes environment. Wait for
    // that short write window instead of making the E2E outcome depend on
    // which writer wins the SQLite lock first.
    sqlite.exec("PRAGMA busy_timeout = 10000");
    sqlite.exec("BEGIN IMMEDIATE");
    transactionOpen = true;
    const environment = sqlite
      .prepare(
        `INSERT INTO task_environments (
           id, task_id, executor_type, executor_id, executor_profile_id,
           control_port, status, workspace_path, container_id, sandbox_id,
           task_dir_name, created_at, updated_at
         ) VALUES (
           ?, ?, 'k8s', ?, ?, 8765, 'ready', '', '', '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
         ) ON CONFLICT(task_id) WHERE retired_at IS NULL DO UPDATE SET
           executor_type = excluded.executor_type,
           executor_id = excluded.executor_id,
           executor_profile_id = excluded.executor_profile_id,
           control_port = excluded.control_port,
           status = excluded.status,
           workspace_path = excluded.workspace_path,
           container_id = excluded.container_id,
           sandbox_id = excluded.sandbox_id,
           task_dir_name = excluded.task_dir_name,
           updated_at = CURRENT_TIMESTAMP
         RETURNING id`,
      )
      .get(environmentId, taskId, executorId, profileId);
    if (typeof environment?.id !== "string") {
      throw new Error("test fixture did not resolve an active environment");
    }
    sqlite
      .prepare("UPDATE task_sessions SET task_environment_id = ? WHERE id = ?")
      .run(environment.id, sessionId);
    sqlite.exec("COMMIT");
    transactionOpen = false;
  } catch (error) {
    if (transactionOpen) sqlite.exec("ROLLBACK");
    throw error;
  } finally {
    sqlite.close();
  }
}
