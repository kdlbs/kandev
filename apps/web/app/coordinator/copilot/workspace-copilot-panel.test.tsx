import type { PageContext } from "./use-page-context";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { createWorkspaceCopilotStore } from "@/hooks/domains/coordinator/workspace-copilot-store";

const mocks = vi.hoisted(() => ({
  useCoordinatorCopilot: vi.fn(),
  useCoordinatorWatches: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, o?: { label?: string }) => (o?.label ? `${key}:${o.label}` : key),
  }),
}));
vi.mock("@/components/routing/app-link", () => ({
  default: ({ children }: { children: React.ReactNode }) => <a href="/x">{children}</a>,
}));
vi.mock("./use-coordinator-copilot", () => ({
  useCoordinatorCopilot: mocks.useCoordinatorCopilot,
}));
vi.mock("./use-coordinator-watches", () => ({
  useCoordinatorWatches: mocks.useCoordinatorWatches,
}));
vi.mock("./coordinator-copilot-panel-content", () => ({
  CoordinatorCopilotPanelContent: (p: {
    chip: { ref: { kind: string; id: string } } | null;
    chipRow: React.ReactNode;
    headerActions: React.ReactNode;
  }) => (
    <div>
      <div data-testid="header">{p.headerActions}</div>
      <div data-testid="chip-ref">{p.chip ? `${p.chip.ref.kind}:${p.chip.ref.id}` : ""}</div>
      {p.chipRow}
    </div>
  ),
}));

import { WorkspaceCopilotPanel } from "./workspace-copilot-panel";

function coordinator(id: string): Coordinator {
  return { id, name: `Name ${id}` } as Coordinator;
}

const NOT_WATCHED = "workspace-copilot-not-watched";

const CONTEXT: PageContext = {
  ref: { kind: "task" as const, id: "t-1" },
  label: "KAN-7",
  workflowId: "wf-1",
  repositoryIds: null,
};

function mount(
  over: { coordinators?: Coordinator[]; routeKey?: string; context?: PageContext } = {},
) {
  const store = createWorkspaceCopilotStore();
  const utils = render(
    <TooltipProvider>
      <WorkspaceCopilotPanel
        store={store}
        workspaceId="ws-1"
        coordinator={coordinator("c1")}
        coordinators={over.coordinators ?? [coordinator("c1")]}
        context={over.context ?? CONTEXT}
        routeKey={over.routeKey ?? "task:t-1"}
        onClose={vi.fn()}
        onSwitch={vi.fn()}
        onGone={vi.fn()}
      />
    </TooltipProvider>,
  );
  return { store, ...utils };
}

beforeEach(() => {
  mocks.useCoordinatorCopilot.mockReturnValue({
    openSequence: { state: { kind: "ready" } },
    launcher: { gone: false },
  });
  mocks.useCoordinatorWatches.mockReturnValue({ loaded: true, watches: undefined });
});
afterEach(cleanup);

describe("WorkspaceCopilotPanel", () => {
  it("runs the controller on the host store", () => {
    const { store } = mount();
    expect(mocks.useCoordinatorCopilot).toHaveBeenCalledWith("ws-1", "c1", true, store);
  });

  it("derives the chip from the page context", () => {
    mount();
    expect(screen.getByTestId("chip-ref").textContent).toBe("task:t-1");
    expect(screen.getByTestId("workspace-copilot-chip-label").textContent).toContain("KAN-7");
  });

  it("removes the chip for the route only", () => {
    const { store } = mount();
    fireEvent.click(screen.getByLabelText("coordinator:copilotRemoveChip"));
    expect(store.getState().chipDismissedFor).toBe("task:t-1");
    expect(screen.queryByTestId("workspace-copilot-chip-row")).toBeNull();
  });

  it("shows the switcher only with two or more coordinators", () => {
    mount();
    expect(screen.queryByTestId("workspace-copilot-switcher")).toBeNull();
    cleanup();
    mount({ coordinators: [coordinator("c1"), coordinator("c2")] });
    expect(screen.getByTestId("workspace-copilot-switcher")).toBeTruthy();
  });

  it("hints when the coordinator does not watch the page workflow", () => {
    mocks.useCoordinatorWatches.mockReturnValue({
      loaded: true,
      watches: { scope: "selected", workflow_ids: ["other"] },
    });
    mount();
    expect(screen.getByTestId(NOT_WATCHED)).toBeTruthy();
  });

  it("offers no hint before the watches have loaded", () => {
    mocks.useCoordinatorWatches.mockReturnValue({ loaded: false, watches: undefined });
    mount();
    expect(screen.queryByTestId(NOT_WATCHED)).toBeNull();
  });

  it("reports the coordinator gone when the controller does", () => {
    const onGone = vi.fn();
    mocks.useCoordinatorCopilot.mockReturnValue({
      openSequence: { state: { kind: "gone" } },
      launcher: { gone: false },
    });
    const store = createWorkspaceCopilotStore();
    act(() => {
      render(
        <TooltipProvider>
          <WorkspaceCopilotPanel
            store={store}
            workspaceId="ws-1"
            coordinator={coordinator("c1")}
            coordinators={[coordinator("c1")]}
            context={CONTEXT}
            routeKey="task:t-1"
            onClose={vi.fn()}
            onSwitch={vi.fn()}
            onGone={onGone}
          />
        </TooltipProvider>,
      );
    });
    expect(onGone).toHaveBeenCalledWith("c1");
  });
});

describe("WorkspaceCopilotPanel projects", () => {
  it("lists the watched projects on a second line", () => {
    mocks.useCoordinatorWatches.mockReturnValue({
      loaded: true,
      watches: {
        scope: "selected",
        workflow_ids: [],
        projects: {
          scope: "selected",
          repository_ids: ["r1"],
          include_no_repository: true,
          names: ["a", "b", "c", "d", "e", "f", "g"],
        },
      },
    });
    mount();
    expect(screen.getByTestId("workspace-copilot-projects")).toBeTruthy();
  });

  it("shows no projects line when the coordinator reports no projects", () => {
    mocks.useCoordinatorWatches.mockReturnValue({
      loaded: true,
      watches: { scope: "all", workflow_ids: [] },
    });
    mount();
    expect(screen.queryByTestId("workspace-copilot-projects")).toBeNull();
  });

  it("hints when the task's repositories are outside the selected projects", () => {
    mocks.useCoordinatorWatches.mockReturnValue({
      loaded: true,
      watches: {
        scope: "all",
        workflow_ids: [],
        projects: { scope: "selected", repository_ids: ["r1"], include_no_repository: false },
      },
    });
    mount({ context: { ...CONTEXT, repositoryIds: ["r2"] } });
    expect(screen.getByTestId(NOT_WATCHED)).toBeTruthy();
    cleanup();
    mount({ context: { ...CONTEXT, repositoryIds: ["r1", "r2"] } });
    expect(screen.queryByTestId(NOT_WATCHED)).toBeNull();
    cleanup();
    mount({ context: { ...CONTEXT, repositoryIds: null } });
    expect(screen.queryByTestId(NOT_WATCHED)).toBeNull();
  });
});
