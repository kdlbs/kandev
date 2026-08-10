import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { RegisteredChangeRequestStatus } from "./registered-change-request-status";

const PLUGIN_ID = "registered-status-test";
const refresh = vi.fn(async () => undefined);
const openDesktop = vi.fn();
const openMobile = vi.fn();
const unlink = vi.fn(async () => undefined);
const toast = vi.fn();

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { addReviewPanel: typeof openDesktop }) => unknown) =>
    selector({ addReviewPanel: openDesktop }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      workspaces: { activeId: "workspace-a" },
      taskPRs: { byTaskId: {} },
      taskMRs: { byWorkspaceId: {} },
      tasks: { activeSessionId: "session-a" },
      setMobileSessionReview: openMobile,
    }),
}));

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

function registerProvider() {
  pluginRegistry.forPlugin(PLUGIN_ID).registerReviewProvider({
    id: "bitbucket",
    label: "Bitbucket",
    changeRequestNoun: "pull request",
    order: 50,
    getSnapshot: () => [
      {
        providerId: "bitbucket",
        reviewKey: "workspace/repo/42",
        title: "Fix shared status",
        url: "https://bitbucket.test/workspace/repo/pull-requests/42",
        repositoryId: "workspace/repo",
        state: "open",
        taskStatus: {
          number: 42,
          state: "open",
          pipelineState: "failure",
          checks: [{ id: "build", label: "Build", state: "failure", detail: "Failed" }],
          review: { state: "approved", approved: 1, required: 1, requested: 0 },
          unresolvedComments: 2,
        },
      },
    ],
    subscribe: () => () => undefined,
    refresh,
    unlink,
    ReviewPanel: () => null,
  });
}

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  refresh.mockClear();
  openDesktop.mockClear();
  openMobile.mockClear();
  unlink.mockClear();
  toast.mockClear();
  vi.useRealTimers();
});

describe("RegisteredChangeRequestStatus", () => {
  it("renders registered status with the shared popover and routes refresh/review", async () => {
    vi.useFakeTimers();
    registerProvider();
    render(<RegisteredChangeRequestStatus taskId="task-a" surface="topbar" />);

    const trigger = screen.getByRole("button", { name: /#42 Fix shared status/ });
    await act(async () => Promise.resolve());
    expect(refresh).toHaveBeenCalledOnce();
    refresh.mockClear();
    fireEvent.mouseEnter(trigger);
    act(() => vi.advanceTimersByTime(150));

    expect(screen.getByTestId("pr-topbar-popover-inner").className).toContain("gap-2");
    expect(screen.getByTestId("pr-checks-progress")).toBeTruthy();
    expect(screen.getByTestId("pr-workflow-row").className).toContain("hover:bg-accent/50");
    expect(screen.getByTestId("pr-review-row").textContent).toContain("Approved");
    expect(screen.getByTestId("pr-comments-row").textContent).toContain("2 unresolved comments");
    await act(async () => Promise.resolve());
    expect(refresh).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByTestId("pr-popover-title"));
    expect(openDesktop).toHaveBeenCalledWith("bitbucket", "workspace/repo/42", "Fix shared status");
  });

  it("uses the composer chip anatomy and disappears on provider unload", async () => {
    registerProvider();
    const { rerender } = render(
      <RegisteredChangeRequestStatus taskId="task-a" surface="composer" />,
    );

    expect(screen.getByTestId("integration-change-request-status-chip")).toBeTruthy();
    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));
    rerender(<RegisteredChangeRequestStatus taskId="task-a" surface="composer" />);
    expect(screen.queryByTestId("integration-change-request-status-chip")).toBeNull();
  });

  it("routes shared unlink through the registered provider with verified UI context", async () => {
    vi.useFakeTimers();
    registerProvider();
    render(<RegisteredChangeRequestStatus taskId="task-a" surface="topbar" />);
    const trigger = screen.getByRole("button", { name: /#42 Fix shared status/ });
    fireEvent.mouseEnter(trigger);
    act(() => vi.advanceTimersByTime(150));

    fireEvent.click(screen.getByRole("button", { name: "Unlink pull request #42" }));
    await act(async () => Promise.resolve());

    expect(unlink).toHaveBeenCalledWith({
      workspaceId: "workspace-a",
      taskId: "task-a",
      reviewKey: "workspace/repo/42",
      signal: expect.any(AbortSignal),
    });
    expect(refresh).toHaveBeenCalled();
  });

  it("deduplicates the 90 second poll across mounted status surfaces", async () => {
    vi.useFakeTimers();
    registerProvider();
    render(
      <>
        <RegisteredChangeRequestStatus taskId="task-a" surface="topbar" />
        <RegisteredChangeRequestStatus taskId="task-a" surface="composer" />
      </>,
    );
    await act(async () => Promise.resolve());
    refresh.mockClear();

    await act(async () => {
      vi.advanceTimersByTime(90_000);
      await Promise.resolve();
    });

    expect(refresh).toHaveBeenCalledOnce();
  });
});
