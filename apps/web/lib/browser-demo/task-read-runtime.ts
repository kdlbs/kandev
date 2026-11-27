import type { FileInfo } from "@/lib/state/slices/session-runtime/types";
import type { DemoState } from "./scenario";

type ReadResult = { payload: unknown; error?: boolean };

export function routeDemoTaskRead(
  state: DemoState,
  action: string,
  input: Record<string, unknown>,
  gitFiles: (sessionId: string) => Record<string, FileInfo>,
): ReadResult | undefined {
  if (action === "task.session.status") return sessionStatus(state, input);
  if (action === "task.walkthrough.get") return { payload: null };
  if (action === "task.preview_feedback.list") {
    return { payload: { task_id: input.task_id, revision: 0, items: [] } };
  }
  if (action === "github.task_pr.sync") {
    return { payload: { prs: state.taskPRs[String(input.task_id)] ?? [], permanent: true } };
  }
  if (action === "github.pr_files.get") {
    const pr = Object.values(state.taskPRs)
      .flat()
      .find(
        (item) =>
          item.owner === input.owner && item.repo === input.repo && item.pr_number === input.number,
      );
    const task = state.tasks.find((item) => item.id === pr?.task_id);
    const files = task?.primary_session_id ? gitFiles(task.primary_session_id) : {};
    return {
      payload: {
        files: Object.values(files).map((file) => ({
          filename: file.path,
          status: file.status,
          additions: file.additions,
          deletions: file.deletions,
          patch: file.diff,
        })),
      },
    };
  }
  return undefined;
}

// i18n-exempt: browser demo transport diagnostics use literal fixture text
function sessionStatus(state: DemoState, input: Record<string, unknown>): ReadResult {
  const session = state.sessions.find(
    (item) => item.id === input.session_id && item.task_id === input.task_id,
  );
  if (!session) return { payload: { message: "Session not found" }, error: true };
  const live = ["IDLE", "RUNNING", "STARTING", "WAITING_FOR_INPUT"].includes(session.state);
  return {
    payload: {
      session_id: session.id,
      task_id: session.task_id,
      state: session.state,
      updated_at: session.updated_at,
      agent_profile_id: session.agent_profile_id,
      is_agent_running: live,
      is_resumable: live,
      needs_resume: false,
      is_idle_suspended: false,
      auto_resume_allowed: false,
      needs_workspace_restore: false,
      is_remote_executor: false,
      capabilities: { embedded_vscode: false },
      worktree_path: session.worktree_path,
      worktree_branch: session.worktree_branch,
    },
  };
}
