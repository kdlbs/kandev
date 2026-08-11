import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import { RegisteredChangeRequestTaskIcon } from "./registered-change-request-task-icon";

const PLUGIN_ID = "task-indicator-test";
const TASK_ID = "task-a";
const TASK_ICON_TEST_ID = "registered-change-request-task-icon-task-a";
const refreshAssociations = vi.fn(async () => undefined);
const refreshReview = vi.fn(async () => undefined);

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
    getSnapshot: (taskId) =>
      taskId === TASK_ID
        ? [
            {
              providerId: "bitbucket",
              reviewKey: "repo#1",
              title: "Provider-neutral contract",
              url: "https://bitbucket.example.test/workspace/repo/pull-requests/1",
              repositoryId: "workspace/repo",
              state: "OPEN",
              taskStatus: {
                number: 1,
                state: "open",
                pipelineState: "success",
                checks: [{ id: "build", label: "Build", state: "success" }],
                review: { state: "approved", approved: 1, required: 1 },
              },
            },
          ]
        : [],
    subscribe: () => () => undefined,
    refresh: refreshReview,
    getAssociationSnapshot: () => [
      { providerId: "bitbucket", taskId: TASK_ID, reviewKey: "repo#1" },
      { providerId: "bitbucket", taskId: TASK_ID, reviewKey: "repo#2" },
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
  refreshReview.mockClear();
});

describe("RegisteredChangeRequestTaskIcon", () => {
  it("matches a renamed review through immutable association identity", async () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerReviewProvider({
      id: "bitbucket",
      label: "Bitbucket",
      changeRequestNoun: "pull request",
      order: 50,
      getSnapshot: () => [
        {
          providerId: "bitbucket",
          reviewKey: "old-name/repo#1",
          title: "Recreated repository at old path",
          url: "https://bitbucket.example.test/old-name/repo/pull-requests/1",
          repositoryId: "different-repo-uuid",
          state: "OPEN",
          taskStatus: {
            number: 1,
            state: "open",
            pipelineState: "failure",
            checks: [],
          },
        },
        {
          providerId: "bitbucket",
          reviewKey: "new-name/repo#1",
          title: "Repository renamed",
          url: "https://bitbucket.example.test/new-name/repo/pull-requests/1",
          repositoryId: "repo-uuid",
          state: "OPEN",
          taskStatus: {
            number: 1,
            state: "open",
            pipelineState: "success",
            checks: [],
          },
        },
      ],
      subscribe: () => () => undefined,
      refresh: async () => undefined,
      getAssociationSnapshot: () => [
        {
          providerId: "bitbucket",
          taskId: TASK_ID,
          reviewKey: "old-name/repo#1",
          repositoryId: "repo-uuid",
          changeRequestNumber: 1,
        },
      ],
      subscribeAssociations: () => () => undefined,
      refreshAssociations: async () => undefined,
      ReviewPanel: () => null,
    });
    render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    fireEvent.pointerEnter(screen.getByTestId(TASK_ICON_TEST_ID), { pointerType: "mouse" });
    await act(async () => Promise.resolve());
    const summary = within(screen.getAllByTestId("pr-task-status-summary")[0]!);
    expect(summary.getByTestId("pr-task-status-title").textContent).toBe("Repository renamed");
    expect(screen.queryByText("Recreated repository at old path")).toBeNull();
    expect(screen.getByTestId(TASK_ICON_TEST_ID).className).toContain("text-green-500");
  });
});

describe("RegisteredChangeRequestTaskIcon shared chrome", () => {
  it("renders one host semantic PR glyph and count from workspace associations", async () => {
    registerProvider();
    render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    const icon = screen.getByTestId(TASK_ICON_TEST_ID);
    expect(icon.getAttribute("aria-label")).toBe("2 Bitbucket pull requests linked");
    expect(icon.textContent).toContain("2");
    expect(refreshAssociations).toHaveBeenCalledOnce();
  });

  it("eagerly hydrates linked-task status and shows the shared structured summary", async () => {
    registerProvider();
    render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    expect(refreshReview).toHaveBeenCalledOnce();
    refreshReview.mockClear();
    fireEvent.pointerEnter(screen.getByTestId(TASK_ICON_TEST_ID), {
      pointerType: "mouse",
    });
    await act(async () => Promise.resolve());

    const summaries = screen.getAllByTestId("pr-task-status-summary");
    const summary = within(summaries[0]);
    expect(summary.getByTestId("pr-task-status-title").textContent).toBe(
      "Provider-neutral contract",
    );
    expect(summary.getByTestId("pr-task-status-review").textContent).toContain("Approved");
    expect(summary.getByTestId("pr-task-status-ci").textContent).toContain("Passed");
    expect(refreshReview).toHaveBeenCalledOnce();
  });

  it("uses the shared semantic status color when provider detail is available", async () => {
    registerProvider();
    render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    expect(screen.getByTestId(TASK_ICON_TEST_ID).className).toContain("text-green-500");
  });

  it("deduplicates one workspace refresh across many task rows and unloads reactively", async () => {
    registerProvider();
    const view = render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
        <RegisteredChangeRequestTaskIcon taskId="task-b" />
      </TooltipProvider>,
    );
    await act(async () => Promise.resolve());

    expect(refreshAssociations).toHaveBeenCalledOnce();
    expect(screen.getByTestId("registered-change-request-task-icon-task-b")).toBeTruthy();
    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));
    view.rerender(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
        <RegisteredChangeRequestTaskIcon taskId="task-b" />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId(TASK_ICON_TEST_ID)).toBeNull();
  });

  it("reuses a settled workspace refresh when task rows mount sequentially", async () => {
    registerProvider();
    const first = render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
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

describe("RegisteredChangeRequestTaskIcon status hydration", () => {
  it("changes from neutral to the published provider status", async () => {
    let publish!: () => void;
    let snapshot: readonly ReviewItemSummary[] = [];
    const listeners = new Set<() => void>();
    pluginRegistry.forPlugin(PLUGIN_ID).registerReviewProvider({
      id: "bitbucket",
      label: "Bitbucket",
      changeRequestNoun: "pull request",
      order: 50,
      getSnapshot: () => snapshot,
      subscribe: (_taskId, listener) => {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
      refresh: () =>
        new Promise<void>((resolve) => {
          publish = () => {
            snapshot = [
              {
                providerId: "bitbucket",
                reviewKey: "repo#1",
                title: "Failing build",
                url: "https://bitbucket.example.test/workspace/repo/pull-requests/1",
                repositoryId: "workspace/repo",
                state: "OPEN",
                taskStatus: {
                  number: 1,
                  state: "open",
                  pipelineState: "failure",
                  checks: [{ id: "build", label: "Build", state: "failure" }],
                },
              },
            ];
            listeners.forEach((listener) => listener());
            resolve();
          };
        }),
      getAssociationSnapshot: () => [
        { providerId: "bitbucket", taskId: TASK_ID, reviewKey: "repo#1" },
      ],
      subscribeAssociations: () => () => undefined,
      refreshAssociations: async () => undefined,
      ReviewPanel: () => null,
    });
    render(
      <TooltipProvider>
        <RegisteredChangeRequestTaskIcon taskId={TASK_ID} />
      </TooltipProvider>,
    );
    await waitFor(() => expect(publish).toBeTypeOf("function"));

    expect(screen.getByTestId(TASK_ICON_TEST_ID).className).toContain("text-muted-foreground");
    await act(async () => publish());
    expect(screen.getByTestId(TASK_ICON_TEST_ID).className).toContain("text-red-500");
  });
});
