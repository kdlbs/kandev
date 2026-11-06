import { createContext, useContext, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";

type TaskWorkspace = { taskId: string; workspaceId: string; sessionId?: string };
const Context = createContext<TaskWorkspace | null>(null);

/** The original task's workspace when its surface is embedded outside workspace navigation. */
export function TaskWorkspaceProvider({
  taskId,
  workspaceId,
  sessionId,
  children,
}: TaskWorkspace & { children: ReactNode }) {
  return <Context.Provider value={{ taskId, workspaceId, sessionId }}>{children}</Context.Provider>;
}

export function useTaskSessionId(taskId: string): string | null {
  const owner = useContext(Context);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  if (owner) return owner.taskId === taskId ? (owner.sessionId ?? null) : null;
  return activeSessionId;
}

export function useTaskWorkspaceId(taskId: string | null): string | null {
  const owner = useContext(Context);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  if (owner) return owner.taskId === taskId ? owner.workspaceId : null;
  return activeWorkspaceId;
}

export function useEmbeddedTaskWorkspace(): TaskWorkspace | null {
  return useContext(Context);
}
