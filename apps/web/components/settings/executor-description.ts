import type { ExecutorType } from "@/lib/types/http";

export function getExecutorDescription(type: ExecutorType): string {
  if (type === "local_pc") return "Runs agents directly in the repository folder.";
  if (type === "worktree") return "Creates git worktrees for isolated agent sessions.";
  if (type === "local_docker") return "Runs Docker containers on this machine.";
  if (type === "remote_docker") return "Connects to a remote Docker host.";
  if (type === "sprites") return "Runs agents in Sprites.dev cloud sandboxes.";
  if (type === "ssh") return "Runs agents on a trusted Linux amd64 or macOS host over SSH.";
  return "Custom executor.";
}
