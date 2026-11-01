import type { TaskNavigationIdentity } from "./task-navigation-reads";

export const ACTIVE_TASK_ID = "task-active";
export const RETAINED_TASK_ID = "task-retained";

export function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

export function identity(taskId: string): TaskNavigationIdentity {
  return {
    task: { id: taskId } as TaskNavigationIdentity["task"],
    allSessionsResponse: { sessions: [], total: 0 },
  };
}

export function temporaryNetworkFailure(): Promise<TaskNavigationIdentity> {
  return Promise.reject(new TypeError("network unavailable"));
}
