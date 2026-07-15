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
  const unlisteners = await Promise.all([
    adapter.listen("close-context", actions["close-context"]),
    adapter.listen("open-settings", actions["open-settings"]),
    adapter.listen("new-task", actions["new-task"]),
  ]);
  return () => {
    for (const unlisten of unlisteners) unlisten();
  };
}
