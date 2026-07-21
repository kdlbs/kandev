import type { DesktopV1Adapter, DesktopUnlisten } from "./adapter";

type DesktopCommandActionDependencies = {
  closeContext: () => void;
  navigate: (href: string) => void;
  requestNewTask: () => void;
};

export type DesktopCommandActions = {
  "close-context": () => void;
  "open-settings": () => void;
  "new-task": () => void;
};

export function createDesktopCommandActions(
  dependencies: DesktopCommandActionDependencies,
): DesktopCommandActions {
  return {
    "close-context": dependencies.closeContext,
    "open-settings": () => dependencies.navigate("/settings/general"),
    "new-task": dependencies.requestNewTask,
  };
}

export async function subscribeDesktopCommandActions(
  adapter: DesktopV1Adapter,
  actions: DesktopCommandActions,
): Promise<DesktopUnlisten> {
  const results = await Promise.allSettled([
    adapter.listen("close-context", actions["close-context"]),
    adapter.listen("open-settings", actions["open-settings"]),
    adapter.listen("new-task", actions["new-task"]),
  ]);
  const failure = results.find(
    (result): result is PromiseRejectedResult => result.status === "rejected",
  );
  if (failure) {
    for (const result of results) {
      if (result.status === "fulfilled") result.value();
    }
    throw failure.reason;
  }
  const unlisteners = results
    .filter(
      (result): result is PromiseFulfilledResult<DesktopUnlisten> => result.status === "fulfilled",
    )
    .map((result) => result.value);
  return () => {
    for (const unlisten of unlisteners) unlisten();
  };
}
