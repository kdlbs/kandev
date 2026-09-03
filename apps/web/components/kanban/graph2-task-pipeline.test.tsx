import type { ComponentProps } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";
import { Graph2TaskPipeline } from "./graph2-task-pipeline";

const routerPush = vi.fn();
vi.mock("@/lib/routing/client-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/routing/client-router")>();
  return {
    ...actual,
    useRouter: () => ({ ...actual.useRouter(), push: routerPush }),
  };
});

afterEach(() => {
  cleanup();
  routerPush.mockClear();
});

const IN_PROGRESS_TITLE = "In Progress";
const moreOptionsLabel = () => t("kanban:moreOptions");

const STEPS: WorkflowStep[] = [
  { id: "step-1", title: "Triage", color: "#888" },
  { id: "step-2", title: IN_PROGRESS_TITLE, color: "#888" },
  { id: "step-3", title: "Done", color: "#888" },
];

function makeTask(workflowStepId: string): Task {
  return {
    id: "task-1",
    title: "A task",
    workflowStepId,
  } as Task;
}

function renderPipeline(
  task: Task,
  steps: WorkflowStep[],
  overrides: Partial<ComponentProps<typeof Graph2TaskPipeline>> = {},
) {
  return render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider delayDuration={0}>
          <Graph2TaskPipeline
            task={task}
            steps={steps}
            moveTargetSteps={steps}
            workspaceId={null}
            externalLinkAvailability={{ jira: false, linear: false, sentry: false }}
            repositories={[]}
            onMoveTask={() => undefined}
            onPreviewTask={() => undefined}
            onOpenTask={() => undefined}
            onDeleteTask={() => undefined}
            {...overrides}
          />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

const WORKFLOW_ID = "wf-1";

/** Seeds `kanbanMulti.snapshots` so the shared menu's move-to-step submenu resolves a current workflow for the task. */
function renderPipelineWithWorkflowSnapshot(
  task: Task,
  steps: WorkflowStep[],
  overrides: Partial<ComponentProps<typeof Graph2TaskPipeline>> = {},
) {
  return render(
    <StateProvider
      initialState={{
        kanbanMulti: {
          snapshots: {
            [WORKFLOW_ID]: {
              workflowId: WORKFLOW_ID,
              workflowName: "Workflow",
              steps: steps.map((s, i) => ({ ...s, position: i })),
              tasks: [{ ...task, workflowId: WORKFLOW_ID, position: 0 }] as never[],
            },
          },
          isLoading: false,
        },
      }}
    >
      <ToastProvider>
        <TooltipProvider delayDuration={0}>
          <Graph2TaskPipeline
            task={task}
            steps={steps}
            moveTargetSteps={steps}
            workspaceId={null}
            externalLinkAvailability={{ jira: false, linear: false, sentry: false }}
            repositories={[]}
            onMoveTask={() => undefined}
            onPreviewTask={() => undefined}
            onOpenTask={() => undefined}
            onDeleteTask={() => undefined}
            {...overrides}
          />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

describe("Graph2TaskPipeline — no resolvable current step (AC-UI-PIPELINE-ROW-005.6)", () => {
  it("renders exactly one labelled unassigned marker and every displayed step as a future pill", () => {
    renderPipeline(makeTask(""), STEPS);

    const marker = screen.getByTestId("graph2-step-node-unassigned");
    expect(marker.textContent).toContain(t("kanban:pipelineUnassignedStep"));

    expect(screen.queryAllByTestId("graph2-step-node-past")).toHaveLength(0);
    expect(screen.getAllByTestId("graph2-step-node-future")).toHaveLength(STEPS.length);
    for (const step of STEPS) {
      expect(screen.queryByRole("button", { name: step.title })).toBeNull();
    }
  });

  it("renders a connector between the unassigned marker and the first step, in the not-yet-reached form, so a 9-step run carries 9 connectors", () => {
    const nineSteps: WorkflowStep[] = Array.from({ length: 9 }, (_, i) => ({
      id: `step-${i}`,
      title: `Step ${i}`,
      color: "#888",
    }));
    renderPipeline(makeTask(""), nineSteps);

    const connectors = screen.getAllByTestId("graph2-connector");
    expect(connectors).toHaveLength(9);
    expect(connectors[0].getAttribute("data-connector-type")).toBe("future");
  });

  it("does not render the unassigned marker when the current step resolves normally", () => {
    renderPipeline(makeTask("step-2"), STEPS);
    expect(screen.queryByTestId("graph2-step-node-unassigned")).toBeNull();
  });

  it("does not render the unassigned marker when there are no displayed steps (AC-UI-PIPELINE-ROW-005.7 takes precedence)", () => {
    renderPipeline(makeTask(""), []);
    expect(screen.queryByTestId("graph2-step-node-unassigned")).toBeNull();
    expect(screen.queryByTestId("graph2-connector")).toBeNull();
  });
});

describe("Graph2TaskPipeline — first/last step move-control boundaries", () => {
  it("shows a single next control at the first step (Triage -> In Progress)", () => {
    renderPipeline(makeTask("step-1"), STEPS);
    fireEvent.mouseEnter(screen.getByRole("button", { name: "Triage" }).parentElement!);
    expect(screen.getByRole("button", { name: "Move to In Progress" })).not.toBeNull();
  });

  it("shows a single prev control at the last step (Done <- In Progress)", () => {
    renderPipeline(makeTask("step-3"), STEPS);
    fireEvent.mouseEnter(screen.getByRole("button", { name: "Done" }).parentElement!);
    expect(screen.getByRole("button", { name: "Move to In Progress" })).not.toBeNull();
    expect(screen.getAllByRole("button", { name: /move to/i })).toHaveLength(1);
  });
});

describe("Graph2TaskPipeline — row-local in-flight move guard (AC-UI-PIPELINE-ROW-005.2)", () => {
  it("disables the step run's move controls while isMoving is true", () => {
    renderPipeline(makeTask("step-2"), STEPS, { isMoving: true });
    fireEvent.mouseEnter(screen.getByRole("button", { name: IN_PROGRESS_TITLE }).parentElement!);

    const moveButtons = screen.getAllByRole("button", { name: /move to/i });
    expect(moveButtons.length).toBeGreaterThan(0);
    for (const button of moveButtons) {
      expect(button.hasAttribute("disabled")).toBe(true);
    }
  });

  it("leaves move controls enabled when isMoving is false", () => {
    renderPipeline(makeTask("step-2"), STEPS, { isMoving: false });
    fireEvent.mouseEnter(screen.getByRole("button", { name: IN_PROGRESS_TITLE }).parentElement!);

    const moveButtons = screen.getAllByRole("button", { name: /move to/i });
    expect(moveButtons.length).toBeGreaterThan(0);
    for (const button of moveButtons) {
      expect(button.hasAttribute("disabled")).toBe(false);
    }
  });

  it("disables the move-to-step menu entries while isMoving is true", async () => {
    renderPipelineWithWorkflowSnapshot(makeTask("step-2"), STEPS, { isMoving: true });

    const trigger = screen.getByRole("button", { name: moreOptionsLabel() });
    fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
    fireEvent.click(trigger);

    await waitFor(() => expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0));
    const moveEntry = screen.getByTestId("task-context-move-to");
    expect(moveEntry.getAttribute("data-disabled")).not.toBeNull();
  });

  it("keeps the task-menu trigger touch-sized on coarse pointers", () => {
    renderPipeline(makeTask("step-2"), STEPS);

    const trigger = screen.getByRole("button", { name: moreOptionsLabel() });
    expect(trigger.className).toMatch(/\[@media\(pointer:coarse\)\]:h-11/);
    expect(trigger.className).toMatch(/\[@media\(pointer:coarse\)\]:w-11/);
  });
});

describe("Graph2TaskPipeline — the row has exactly one activation decision", () => {
  it("routes a plain click on the labelled current-step pill through the row's own handler exactly once, preview-aware like the Kanban card's body click", () => {
    const onOpenTask = vi.fn();
    const onPreviewTask = vi.fn();
    renderPipeline(makeTask("step-2"), STEPS, { onOpenTask, onPreviewTask });

    fireEvent.click(screen.getByRole("button", { name: IN_PROGRESS_TITLE }));

    // The row's single activation decision calls the preview-aware callback —
    // matching the Kanban card's own body-click wiring
    // (onClick={onPreviewTask} in virtualized-column-task-list.tsx) so "Open
    // preview on click" is respected. `onOpenTask` is reserved for the
    // explicit full-page action offered elsewhere (e.g. a future menu entry),
    // never called directly from the pill or row body.
    expect(onPreviewTask).toHaveBeenCalledTimes(1);
    expect(onOpenTask).not.toHaveBeenCalled();
    expect(routerPush).not.toHaveBeenCalled();
  });
});

describe("Graph2TaskPipeline — row menu open state (isProcessing does not force-open)", () => {
  it("does not pop the row's own menu open when a delete/archive starts from elsewhere (e.g. the context menu)", () => {
    const { rerender } = renderPipeline(makeTask("step-2"), STEPS, { isDeleting: false });

    expect(screen.queryByRole("menu")).toBeNull();

    // Deletion starting from outside this row's dropdown (context menu,
    // multi-select bulk action) flips isDeleting without the user ever
    // clicking this row's own trigger.
    rerender(
      <StateProvider>
        <ToastProvider>
          <TooltipProvider delayDuration={0}>
            <Graph2TaskPipeline
              task={makeTask("step-2")}
              steps={STEPS}
              moveTargetSteps={STEPS}
              workspaceId={null}
              externalLinkAvailability={{ jira: false, linear: false, sentry: false }}
              repositories={[]}
              onMoveTask={() => undefined}
              onPreviewTask={() => undefined}
              onOpenTask={() => undefined}
              onDeleteTask={() => undefined}
              isDeleting
            />
          </TooltipProvider>
        </ToastProvider>
      </StateProvider>,
    );

    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("keeps the menu open through processing once the user did open it", async () => {
    const { rerender } = renderPipeline(makeTask("step-2"), STEPS, { isDeleting: false });

    const trigger = screen.getByRole("button", { name: moreOptionsLabel() });
    fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0));

    rerender(
      <StateProvider>
        <ToastProvider>
          <TooltipProvider delayDuration={0}>
            <Graph2TaskPipeline
              task={makeTask("step-2")}
              steps={STEPS}
              moveTargetSteps={STEPS}
              workspaceId={null}
              externalLinkAvailability={{ jira: false, linear: false, sentry: false }}
              repositories={[]}
              onMoveTask={() => undefined}
              onPreviewTask={() => undefined}
              onOpenTask={() => undefined}
              onDeleteTask={() => undefined}
              isDeleting
            />
          </TooltipProvider>
        </ToastProvider>
      </StateProvider>,
    );

    expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0);
  });
});

describe("Graph2TaskPipeline — out-of-band step change with an open menu (AC-UI-PIPELINE-ROW-005.4)", () => {
  it("keeps the menu open across a re-render at a new step, keyed on task id", async () => {
    const { rerender } = renderPipeline(makeTask("step-1"), STEPS);

    const trigger = screen.getByRole("button", { name: moreOptionsLabel() });
    fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0));

    rerender(
      <StateProvider>
        <ToastProvider>
          <TooltipProvider delayDuration={0}>
            <Graph2TaskPipeline
              task={makeTask("step-2")}
              steps={STEPS}
              moveTargetSteps={STEPS}
              workspaceId={null}
              externalLinkAvailability={{ jira: false, linear: false, sentry: false }}
              repositories={[]}
              onMoveTask={() => undefined}
              onPreviewTask={() => undefined}
              onOpenTask={() => undefined}
              onDeleteTask={() => undefined}
            />
          </TooltipProvider>
        </ToastProvider>
      </StateProvider>,
    );

    expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0);
    // The row sits behind the open menu's aria-hidden background; query with hidden: true.
    expect(screen.getByRole("button", { name: IN_PROGRESS_TITLE, hidden: true })).not.toBeNull();
  });
});

describe("Graph2TaskPipeline — accessible row-position summary (AC-UI-PIPELINE-ROW-001.6)", () => {
  it("names the current step title and its ordinal in the visible run", () => {
    renderPipeline(makeTask("step-2"), STEPS);

    const summary = screen.getByTestId("pipeline-row-position-summary");
    expect(summary.textContent).toBe(
      t("kanban:pipelineRowPosition", { title: IN_PROGRESS_TITLE, position: 2, total: 3 }),
    );
  });

  it("names the unassigned state in place of a step title and ordinal (AC-UI-PIPELINE-ROW-005.6)", () => {
    renderPipeline(makeTask(""), STEPS);

    const summary = screen.getByTestId("pipeline-row-position-summary");
    expect(summary.textContent).toBe(t("kanban:pipelineUnassignedStep"));
  });

  it("renders no summary when there is no displayed step run (AC-UI-PIPELINE-ROW-005.7)", () => {
    renderPipeline(makeTask(""), []);

    expect(screen.queryByTestId("pipeline-row-position-summary")).toBeNull();
  });
});

describe("Graph2TaskPipeline — overflow yield order and scroll terminus (AC-UI-PIPELINE-ROW-003.11)", () => {
  const OVERFLOW_X_AUTO = "overflow-x-auto";
  const observerEntries: Array<{ element: Element; callback: ResizeObserverCallback }> = [];

  class CapturingResizeObserver {
    private readonly callback: ResizeObserverCallback;

    constructor(callback: ResizeObserverCallback) {
      this.callback = callback;
    }

    observe(element: Element) {
      observerEntries.push({ element, callback: this.callback });
    }

    disconnect() {}
    unobserve() {}
  }

  function setGeometry(
    element: Element,
    overrides: { scrollWidth?: number; clientWidth?: number },
  ) {
    if (overrides.scrollWidth !== undefined) {
      Object.defineProperty(element, "scrollWidth", {
        configurable: true,
        value: overrides.scrollWidth,
      });
    }
    if (overrides.clientWidth !== undefined) {
      Object.defineProperty(element, "clientWidth", {
        configurable: true,
        value: overrides.clientWidth,
      });
    }
  }

  function fireResize(element: Element) {
    for (const entry of observerEntries) {
      if (entry.element === element) {
        act(() => entry.callback([], {} as ResizeObserver));
      }
    }
  }

  beforeEach(() => {
    observerEntries.length = 0;
    vi.stubGlobal("ResizeObserver", CapturingResizeObserver);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps the status strip outside the scroll region while it fits (stage 2: the step run scrolls alone)", () => {
    renderPipeline(makeTask("step-2"), STEPS);
    const region = screen.getByTestId("pipeline-row-overflow-region");
    const strip = screen.getByTestId("pipeline-row-status-strip");
    setGeometry(region, { clientWidth: 400 });
    setGeometry(strip, { scrollWidth: 50 });
    fireResize(region);

    expect(region.className).not.toContain(OVERFLOW_X_AUTO);
    expect(screen.getByTestId("pipeline-step-run-scroll").className).toContain(OVERFLOW_X_AUTO);
  });

  it("widens the same region to carry the status strip once it no longer fits alone (the terminus)", () => {
    renderPipeline(makeTask("step-2"), STEPS);
    const region = screen.getByTestId("pipeline-row-overflow-region");
    const strip = screen.getByTestId("pipeline-row-status-strip");
    setGeometry(region, { clientWidth: 40 });
    setGeometry(strip, { scrollWidth: 50 });
    fireResize(region);

    expect(region.className).toContain(OVERFLOW_X_AUTO);
    expect(screen.getByTestId("pipeline-step-run-scroll").className).not.toContain(OVERFLOW_X_AUTO);
  });

  it("holds the information column at one fixed width instead of sizing it to the row's own content", () => {
    renderPipeline(makeTask("step-2"), STEPS);

    const info = screen.getByTestId("pipeline-row-info");
    // A column that grows or shrinks with its own row's content starts each
    // row's step run at a different x, so the runs stop reading as one track
    // down the board (AC-UI-PIPELINE-ROW-003.13).
    expect(info.className).toContain("w-[200px]");
    expect(info.className).toContain("shrink-0");
    expect(info.style.flex).toBe("");
  });
});

describe("Graph2TaskPipeline — the row's information column (AC-UI-PIPELINE-ROW-003.12)", () => {
  const REPOSITORIES = [{ id: "repo-1", workspace_id: "ws-1", name: "alpha" }] as never;

  function infoTask(): Task {
    return {
      id: "task-1",
      title: "A task",
      workflowStepId: "step-2",
      sessionCount: 2,
      updatedAt: new Date().toISOString(),
      repositoryId: "repo-1",
      repositories: [{ id: "link-1", repository_id: "repo-1", position: 0 }],
    } as unknown as Task;
  }

  it("stacks the title, repository, relative time and session count inside the column", () => {
    renderPipeline(infoTask(), STEPS, { repositories: REPOSITORIES });

    const info = screen.getByTestId("pipeline-row-info");
    expect(info.contains(screen.getByTestId("pipeline-row-title"))).toBe(true);
    expect(info.contains(screen.getByTestId("task-repo-chip"))).toBe(true);
    expect(info.contains(screen.getByText(t("kanban:sessionCount", { count: 2 })))).toBe(true);
    expect(info.textContent).toContain("A task");
  });

  it("shows a session count of one, which the card's badge threshold would drop", () => {
    const task = { ...infoTask(), sessionCount: 1 } as Task;
    renderPipeline(task, STEPS, { repositories: REPOSITORIES });

    const info = screen.getByTestId("pipeline-row-info");
    expect(info.textContent).toContain(t("kanban:sessionCount", { count: 1 }));
  });

  it("renders no session count line entry when the task has no sessions", () => {
    const task = { ...infoTask(), sessionCount: 0 } as Task;
    renderPipeline(task, STEPS, { repositories: REPOSITORIES });

    expect(screen.queryByText(t("kanban:sessionCount", { count: 0 }))).toBeNull();
  });
});

describe("Graph2TaskPipeline — plugin card slots on the row (AC-UI-PIPELINE-ROW-003.6)", () => {
  const PLUGIN_ID = "kandev-plugin-row-regression-fixture";

  function Indicator() {
    return <span data-testid="row-regression-indicator">indicator</span>;
  }
  function Tags() {
    return <span data-testid="row-regression-tags">tags</span>;
  }

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  it("renders the registered task-card-indicators plugin component", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerComponent("task-card-indicators", Indicator);

    renderPipeline(makeTask("step-2"), STEPS);

    expect(screen.getByTestId("row-regression-indicator")).not.toBeNull();
  });

  it("does not render the task-card-tags slot, which the row has no lane to hold", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerComponent("task-card-indicators", Indicator);
    pluginRegistry.forPlugin(PLUGIN_ID).registerComponent("task-card-tags", Tags);

    renderPipeline(makeTask("step-2"), STEPS);

    // task-card-tags is contractually spacious — its own row on the Kanban
    // card. The pipeline row is a single line with a fixed information column
    // and a step run, so the only way to seat a tag here is to crop it into a
    // half-legible sliver. The row omits the slot and the card keeps it.
    expect(screen.getByTestId("row-regression-indicator")).not.toBeNull();
    expect(screen.queryByTestId("row-regression-tags")).toBeNull();
  });
});

describe("Graph2TaskPipeline — multi-select mode hides the actions cluster (AC-UI-PIPELINE-ROW-002.8)", () => {
  it("does not render the row menu trigger in multi-select mode", () => {
    renderPipeline(makeTask("step-2"), STEPS, { isMultiSelectMode: true });

    expect(screen.queryByRole("button", { name: moreOptionsLabel() })).toBeNull();
  });

  it("renders the row menu trigger outside multi-select mode", () => {
    renderPipeline(makeTask("step-2"), STEPS);

    expect(screen.getByRole("button", { name: moreOptionsLabel() })).not.toBeNull();
  });
});
