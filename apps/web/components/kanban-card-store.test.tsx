import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { KanbanCard } from "./kanban-card";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("KanbanCard repository state", () => {
  it("renders before the active workspace repository list is hydrated", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    expect(() =>
      render(
        <ToastProvider>
          <StateProvider
            initialState={{
              workspaces: { ...defaultState.workspaces, activeId: "workspace-1" },
              repositories: { ...defaultState.repositories, itemsByWorkspaceId: {} },
            }}
          >
            <KanbanCard
              task={{ id: "task-1", title: "Task", workflowStepId: "step-1" }}
              workspaceId="workspace-1"
              externalLinkAvailability={{ jira: false, linear: false, sentry: false }}
            />
          </StateProvider>
        </ToastProvider>,
      ),
    ).not.toThrow();

    expect(consoleError).not.toHaveBeenCalledWith(
      expect.stringContaining("The result of getSnapshot should be cached"),
    );
  });
});
