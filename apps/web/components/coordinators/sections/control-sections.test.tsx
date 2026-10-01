import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ControlDraft } from "@/lib/coordinators/control-draft";

const summaryMock = vi.fn();
const boardsMock = vi.fn();
const projectsMock = vi.fn();

vi.mock("@/hooks/domains/coordinator/use-action-summary", () => ({
  useActionSummary: () => summaryMock(),
}));
vi.mock("@/hooks/domains/coordinator/use-workspace-boards", () => ({
  useWorkspaceBoards: () => boardsMock(),
}));

vi.mock("@/hooks/domains/coordinator/use-workspace-projects", () => ({
  useWorkspaceProjects: () => projectsMock(),
}));

import { MayDoSection } from "./may-do-section";
import { WatchesSection } from "./watches-section";

const draft = (over: Partial<ControlDraft> = {}): ControlDraft => ({
  actions: {
    create_task: "denied",
    start_agent: "denied",
    message: "denied",
    move: "denied",
    resume: "denied",
    stop: "denied",
  },
  watches: { scope: "all", workflowIds: [] },
  projects: null,
  ...over,
});

function control(d: ControlDraft | null, status = "ready") {
  return {
    draft: d,
    stored: d,
    status,
    retry: vi.fn(),
    fieldError: null,
    setAction: vi.fn(),
    setWatches: vi.fn(),
    setProjects: vi.fn(),
    policyDirty: false,
    watchesDirty: false,
    invalid: false,
  } as never;
}

const counts = (approved: number, rejected: number) => ({ approved, rejected });
const allCounts = () => ({
  create_task: counts(3, 1),
  start_agent: counts(0, 0),
  message: counts(0, 0),
  move: counts(0, 0),
  resume: counts(0, 0),
  stop: counts(0, 0),
});

beforeEach(() => {
  summaryMock.mockReturnValue({ counts: allCounts(), status: "ready", retry: vi.fn() });
  projectsMock.mockReturnValue({
    sets: [{ kind: "repository_set", id: "set-1", name: "Payments", repositoryCount: 2 }],
    loose: [],
    status: "ready",
    retry: vi.fn(),
  });
  boardsMock.mockReturnValue({
    boards: [
      { id: "a", name: "Alpha", hidden: false },
      { id: "b", name: "Beta", hidden: true },
    ],
    status: "ready",
    retry: vi.fn(),
  });
});
afterEach(cleanup);

describe("MayDoSection", () => {
  it("renders six rows in order with the always-human rows and per-class counts", () => {
    render(
      <MayDoSection workspaceId="w1" coordinatorId="c1" canManage control={control(draft())} />,
    );
    const rows = screen.getAllByTestId(/^may-do-row-/).map((r) => r.getAttribute("data-testid"));
    expect(rows).toEqual(
      ["create_task", "start_agent", "message", "move", "resume", "stop"].map(
        (a) => `may-do-row-${a}`,
      ),
    );
    expect(screen.getByText("Merge a pull request")).toBeTruthy();
    expect(screen.getByText("Move a task to Done")).toBeTruthy();
    expect(screen.getByTestId("may-do-counts-create_task").textContent).toContain(
      "3 approved, 1 rejected",
    );
    expect(screen.getAllByText("Nothing yet")).toHaveLength(5);
    expect(screen.getByTestId("may-do-review-message").getAttribute("href")).toBe(
      "/workspaces/w1/coordinator/c1/queue?class=message",
    );
  });

  it("locks Stop to Denied and disables Automatic everywhere", () => {
    render(
      <MayDoSection workspaceId="w1" coordinatorId="c1" canManage control={control(draft())} />,
    );
    expect(screen.getByText("Stopping is not available yet.")).toBeTruthy();
    expect((document.getElementById("may-do-stop-approval") as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect(
      (document.getElementById("may-do-message-automatic") as HTMLButtonElement).disabled,
    ).toBe(true);
  });

  it("changes a setting and shows the start-agent note only when not denied", () => {
    const c = control(draft({ actions: { ...draft().actions, start_agent: "requires_approval" } }));
    render(<MayDoSection workspaceId="w1" coordinatorId="c1" canManage control={c} />);
    expect(screen.getByTestId("may-do-start-agent-note")).toBeTruthy();
    fireEvent.click(document.getElementById("may-do-message-approval") as HTMLElement);
    expect((c as { setAction: ReturnType<typeof vi.fn> }).setAction).toHaveBeenCalledWith(
      "message",
      "requires_approval",
    );
  });

  it("disables every radio for a reader", () => {
    render(
      <MayDoSection
        workspaceId="w1"
        coordinatorId="c1"
        canManage={false}
        control={control(draft())}
      />,
    );
    expect((document.getElementById("may-do-message-denied") as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect((document.getElementById("may-do-message-approval") as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("shows a skeleton while loading and Try again on a failed first read", () => {
    const { rerender } = render(
      <MayDoSection
        workspaceId="w1"
        coordinatorId="c1"
        canManage
        control={control(null, "loading")}
      />,
    );
    expect(screen.getByTestId("may-do-loading")).toBeTruthy();
    const failed = control(null, "error");
    rerender(<MayDoSection workspaceId="w1" coordinatorId="c1" canManage control={failed} />);
    fireEvent.click(screen.getByText("Try again"));
    expect((failed as { retry: ReturnType<typeof vi.fn> }).retry).toHaveBeenCalled();
  });

  it("shows the last-30-days failure without hiding the rows", () => {
    summaryMock.mockReturnValue({ counts: null, status: "error", retry: vi.fn() });
    render(
      <MayDoSection workspaceId="w1" coordinatorId="c1" canManage control={control(draft())} />,
    );
    expect(screen.getByTestId("may-do-summary-failed")).toBeTruthy();
    expect(screen.queryByText("Nothing yet")).toBeNull();
    expect(screen.getByTestId("may-do-row-message")).toBeTruthy();
  });
});

describe("WatchesSection", () => {
  it("turning the every-board switch off starts from every board in order", async () => {
    const c = control(draft());
    render(<WatchesSection workspaceId="w1" canManage control={c} />);
    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() =>
      expect((c as { setWatches: ReturnType<typeof vi.fn> }).setWatches).toHaveBeenCalledWith({
        scope: "selected",
        workflowIds: ["a", "b"],
      }),
    );
  });

  it("lists boards with a Hidden tag and toggles scope", () => {
    const c = control(draft({ watches: { scope: "selected", workflowIds: ["a", "b"] } }));
    render(<WatchesSection workspaceId="w1" canManage control={c} />);
    expect(screen.getByText("Hidden")).toBeTruthy();
    fireEvent.click(screen.getByTestId("watches-toggle-b"));
    expect((c as { setWatches: ReturnType<typeof vi.fn> }).setWatches).toHaveBeenCalledWith({
      scope: "selected",
      workflowIds: ["a"],
    });
  });

  it("refuses to take the last board out of scope", () => {
    render(
      <WatchesSection
        workspaceId="w1"
        canManage
        control={control(draft({ watches: { scope: "selected", workflowIds: ["a"] } }))}
      />,
    );
    expect((screen.getByTestId("watches-toggle-a") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Keep at least one board in scope.")).toBeTruthy();
  });

  it("stops adding at 50 boards", () => {
    const ids = Array.from({ length: 50 }, (_, i) => `w${i}`);
    boardsMock.mockReturnValue({
      boards: [...ids, "extra"].map((id) => ({ id, name: id, hidden: false })),
      status: "ready",
      retry: vi.fn(),
    });
    render(
      <WatchesSection
        workspaceId="w1"
        canManage
        control={control(draft({ watches: { scope: "selected", workflowIds: ids } }))}
      />,
    );
    expect((screen.getByTestId("watches-toggle-extra") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("At most 50 boards can be watched.")).toBeTruthy();
  });

  it("says a workspace with no boards has none to choose from", () => {
    boardsMock.mockReturnValue({ boards: [], status: "ready", retry: vi.fn() });
    render(<WatchesSection workspaceId="w1" canManage control={control(draft())} />);
    expect(screen.getByText("This workspace has no boards to choose from.")).toBeTruthy();
  });

  it("renders no Projects part when the draft has no projects member", () => {
    render(<WatchesSection workspaceId="w1" canManage control={control(draft())} />);
    expect(screen.queryByTestId("watches-projects")).toBeNull();
  });

  it("renders the Projects part for a selected scope and wires a toggle to setProjects", () => {
    const c = control(
      draft({
        projects: {
          scope: "selected",
          entries: [{ kind: "repository_set", id: "set-1" }],
          includeNoRepository: false,
        },
      }),
    );
    render(<WatchesSection workspaceId="w1" canManage control={c} />);
    expect(screen.getByTestId("watches-projects")).toBeTruthy();
    fireEvent.click(screen.getByTestId("watches-project-toggle-set-1"));
    expect((c as { setProjects: ReturnType<typeof vi.fn> }).setProjects).toHaveBeenCalledWith({
      scope: "selected",
      entries: [],
      includeNoRepository: false,
    });
  });

  it("disables the controls for a reader", () => {
    render(
      <WatchesSection
        workspaceId="w1"
        canManage={false}
        control={control(draft({ watches: { scope: "selected", workflowIds: ["a", "b"] } }))}
      />,
    );
    expect((screen.getByTestId("watches-toggle-b") as HTMLButtonElement).disabled).toBe(true);
  });
});
