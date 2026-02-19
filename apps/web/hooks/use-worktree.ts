import { useAppStore } from "@/components/state-provider";

export function useWorktree(worktreeId: string | null) {
  return useAppStore((state) => (worktreeId ? (state.worktrees.items[worktreeId] ?? null) : null));
}
