import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { defaultState } from "@/lib/state/default-state";
import { MobileReposSection } from "./mobile-repos-section";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("MobileReposSection", () => {
  it("renders task repositories before repository details and sessions hydrate", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    expect(() =>
      render(
        <ToastProvider>
          <StateProvider
            initialState={{
              kanban: {
                workflowId: "workflow-1",
                steps: [],
                tasks: [
                  {
                    id: "task-1",
                    workflowStepId: "step-1",
                    title: "Task",
                    position: 0,
                    repositories: [
                      {
                        id: "task-repository-1",
                        repository_id: "repository-1",
                        base_branch: "main",
                        position: 0,
                      },
                      {
                        id: "task-repository-2",
                        repository_id: "repository-2",
                        base_branch: "main",
                        position: 1,
                      },
                    ],
                  },
                ],
              },
              repositories: {
                ...defaultState.repositories,
                itemsByWorkspaceId: {},
              },
              taskSessionsByTask: {
                ...defaultState.taskSessionsByTask,
                itemsByTaskId: {},
              },
            }}
          >
            <MobileReposSection
              taskId="task-1"
              workspaceId="workspace-1"
              onClose={() => undefined}
            />
          </StateProvider>
        </ToastProvider>,
      ),
    ).not.toThrow();

    expect(consoleError).not.toHaveBeenCalledWith(
      expect.stringContaining("The result of getSnapshot should be cached"),
    );
    expect(screen.getByTestId("mobile-repo-row-repository-1")).toBeTruthy();
    expect(screen.getByTestId("mobile-repo-row-repository-2")).toBeTruthy();
  });
});
