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
  const environmentId = `environment-${taskId}`;
  const sqlite = new DatabaseSync(database);
  let transactionOpen = false;
  try {
    sqlite.exec("BEGIN IMMEDIATE");
    transactionOpen = true;
    sqlite
      .prepare(
        `INSERT INTO task_environments (
           id, task_id, executor_type, executor_id, executor_profile_id,
           control_port, status, workspace_path, container_id, sandbox_id,
           task_dir_name, created_at, updated_at
         ) VALUES (
           ?, ?, 'k8s', ?, ?, 8765, 'ready', '', '', '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
         ) ON CONFLICT(task_id) DO UPDATE SET
           executor_type = excluded.executor_type,
           executor_id = excluded.executor_id,
           executor_profile_id = excluded.executor_profile_id,
           control_port = excluded.control_port,
           status = excluded.status,
           workspace_path = excluded.workspace_path,
           container_id = excluded.container_id,
           sandbox_id = excluded.sandbox_id,
           task_dir_name = excluded.task_dir_name,
           updated_at = CURRENT_TIMESTAMP`,
      )
      .run(environmentId, taskId, executorId, profileId);
    sqlite
      .prepare("UPDATE task_sessions SET task_environment_id = ? WHERE id = ?")
      .run(environmentId, sessionId);
    sqlite.exec("COMMIT");
    transactionOpen = false;
  } catch (error) {
    if (transactionOpen) sqlite.exec("ROLLBACK");
    throw error;
  } finally {
    sqlite.close();
  }
}
