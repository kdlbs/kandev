import { createContext } from "react";
import type {
  PluginTaskCreatedHandler,
  PluginTaskCreatedIdentity,
  RegisterPluginTaskCreatedHandler,
} from "@kandev/plugin-sdk";

type CreatedTaskIdentity = PluginTaskCreatedIdentity;
export type TaskCreatedHandler = PluginTaskCreatedHandler;
export type RegisterTaskCreatedHandler = RegisterPluginTaskCreatedHandler;
export interface TaskCreatedHandlerRegistry {
  register: RegisterTaskCreatedHandler;
  notify(task: CreatedTaskIdentity): void;
}

export const TaskCreateDialogTaskCreatedContext = createContext<RegisterTaskCreatedHandler | null>(
  null,
);

export function createTaskCreatedHandlerRegistry(): TaskCreatedHandlerRegistry {
  const handlers = new Set<TaskCreatedHandler>();
  return {
    register(handler: TaskCreatedHandler) {
      handlers.add(handler);
      return () => handlers.delete(handler);
    },
    notify(task: CreatedTaskIdentity) {
      const identity = Object.freeze({ id: task.id, workspace_id: task.workspace_id });
      const logFailure = (error: unknown) => {
        console.error("[plugins] Task-create completion handler failed", error);
      };
      for (const handler of handlers) {
        try {
          void Promise.resolve(handler(identity)).catch(logFailure);
        } catch (error) {
          logFailure(error);
        }
      }
    },
  };
}

export function notifyTaskCreatedHandlers(
  registry: TaskCreatedHandlerRegistry,
  task: CreatedTaskIdentity,
  mode: "create" | "edit",
) {
  if (mode !== "create") return;
  registry.notify(task);
}
