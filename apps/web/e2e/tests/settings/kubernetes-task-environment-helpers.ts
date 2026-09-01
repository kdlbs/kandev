import { execFileSync } from "node:child_process";

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
  execFileSync("sqlite3", [
    database,
    `INSERT INTO task_environments (
       id, task_id, executor_type, executor_id, executor_profile_id,
       control_port, status, workspace_path, container_id, sandbox_id,
       task_dir_name, created_at, updated_at
     ) VALUES (
       '${environmentId}', '${taskId}', 'k8s', '${executorId}', '${profileId}',
       8765, 'ready', '', '', '', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
     );
     UPDATE task_sessions SET task_environment_id = '${environmentId}' WHERE id = '${sessionId}';`,
  ]);
}
