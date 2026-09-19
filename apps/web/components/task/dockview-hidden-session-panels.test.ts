import { afterEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  clearHiddenSessionPanel,
  hiddenSessionIdsFor,
  hideSessionPanel,
} from "./dockview-hidden-session-panels";

function makeApi(panelIds: string[] = []): {
  api: DockviewApi;
  removePanel: ReturnType<typeof vi.fn>;
} {
  const panels = panelIds.map((id) => ({ id }));
  const removePanel = vi.fn((panel: { id: string }) => {
    const index = panels.indexOf(panel);
    if (index >= 0) panels.splice(index, 1);
  });
  return {
    api: {
      panels,
      getPanel: (id: string) => panels.find((panel) => panel.id === id) ?? null,
      removePanel,
    } as unknown as DockviewApi,
    removePanel,
  };
}

afterEach(() => {
  useDockviewStore.setState({ currentLayoutEnvId: null });
  window.sessionStorage.clear();
});

describe("hidden session panels", () => {
  it("lazily loads an environment once and keeps environments independent", () => {
    const { api } = makeApi();
    useDockviewStore.setState({ currentLayoutEnvId: "env-a" });
    hideSessionPanel(api, "session-a", "task-a");

    expect(hiddenSessionIdsFor(api)).toEqual(new Set(["session-a"]));
    useDockviewStore.setState({ currentLayoutEnvId: "env-b" });
    expect(hiddenSessionIdsFor(api)).toEqual(new Set());
  });

  it("persists a hide and removes the live Dockview panel", () => {
    const { api, removePanel } = makeApi(["session:session-a"]);
    useDockviewStore.setState({ currentLayoutEnvId: "env-a" });

    hideSessionPanel(api, "session-a", "task-a");

    expect(hiddenSessionIdsFor(api)).toEqual(new Set(["session-a"]));
    expect(removePanel).toHaveBeenCalledWith(expect.objectContaining({ id: "session:session-a" }));
  });

  it("clears a hidden session and persists the removal", () => {
    const { api } = makeApi();
    useDockviewStore.setState({ currentLayoutEnvId: "env-a" });
    hideSessionPanel(api, "session-a", "task-a");

    clearHiddenSessionPanel(api, "session-a");

    expect(hiddenSessionIdsFor(api)).toEqual(new Set());
  });
});
