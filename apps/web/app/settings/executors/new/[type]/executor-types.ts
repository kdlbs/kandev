// Registry of executor types presented in the "new executor profile" flow.
// Keep entries here (not inline in the page) so the page file stays under
// the 600-line lint cap and new types can be added without touching layout.

export type ExecutorTypeInfo = {
  executorId: string;
  label: string;
  description: string;
};

export const EXECUTOR_TYPE_MAP: Record<string, ExecutorTypeInfo> = {
  local: {
    executorId: "exec-local",
    label: "Local",
    description: "Runs agents directly in the repository folder.",
  },
  worktree: {
    executorId: "exec-worktree",
    label: "Worktree",
    description: "Creates git worktrees for isolated agent sessions.",
  },
  local_docker: {
    executorId: "exec-local-docker",
    label: "Docker",
    description: "Runs Docker containers on this machine.",
  },
  remote_docker: {
    executorId: "exec-remote-docker",
    label: "Remote Docker",
    description: "Connects to a remote Docker host.",
  },
  sprites: {
    executorId: "exec-sprites",
    label: "Sprites.dev",
    description: "Runs agents in Sprites.dev cloud sandboxes.",
  },
  ssh: {
    executorId: "exec-ssh",
    label: "SSH",
    description: "Connects to a remote host over SSH and runs agentctl there.",
  },
};
