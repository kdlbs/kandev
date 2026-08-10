import { act, cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { RegisteredChangeRequestTaskIcon } from "./registered-change-request-task-icon";

const PLUGIN_ID = "task-indicator-test";
const refreshAssociations = vi.fn(async () => undefined);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({ workspaces: { activeId: "workspace-a" } }),
}));

function registerProvider() {
  pluginRegistry.forPlugin(PLUGIN_ID).registerReviewProvider({
    id: "bitbucket",
    label: "Bitbucket",
    changeRequestNoun: "pull request",
    order: 50,
    getSnapshot: () => [],
    subscribe: () => () => undefined,
    refresh: async () => undefined,
    getAssociationSnapshot: () => [
      { providerId: "bitbucket", taskId: "task-a", reviewKey: "repo#1" },
      { providerId: "bitbucket", taskId: "task-a", reviewKey: "repo#2" },
      { providerId: "bitbucket", taskId: "task-b", reviewKey: "repo#3" },
    ],
    subscribeAssociations: () => () => undefined,
    refreshAssociations,
    ReviewPanel: () => null,
  });
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  refreshAssociations.mockClear();
});

describe("RegisteredChangeRequestTaskIcon", () => {
  it("renders one host semantic PR glyph and count from workspace associations", async () => {
    registerProvider();
    render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId="task-a" />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    const icon = screen.getByTestId("registered-change-request-task-icon-task-a");
    expect(icon.getAttribute("aria-label")).toBe("2 Bitbucket pull requests linked");
    expect(icon.textContent).toContain("2");
    expect(refreshAssociations).toHaveBeenCalledOnce();
  });

  it("deduplicates one workspace refresh across many task rows and unloads reactively", async () => {
    registerProvider();
    const view = render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId="task-a" />
        <RegisteredChangeRequestTaskIcon taskId="task-b" />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    expect(refreshAssociations).toHaveBeenCalledOnce();
    expect(screen.getByTestId("registered-change-request-task-icon-task-b")).toBeTruthy();
    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));
    view.rerender(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId="task-a" />
        <RegisteredChangeRequestTaskIcon taskId="task-b" />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId("registered-change-request-task-icon-task-a")).toBeNull();
  });

  it("reuses a settled workspace refresh when task rows mount sequentially", async () => {
    registerProvider();
    const first = render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId="task-a" />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());
    first.unmount();
    render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId="task-b" />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    expect(refreshAssociations).toHaveBeenCalledOnce();
  });
});
