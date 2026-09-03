import { act, cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { getViewByStoredValue } from "./view-registry";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

// AC-002.5: every VIEW_REGISTRY entry's ordering path must actually consume
// `userSettings.kanbanSort` from the store, not a hardcoded comparator — this
// is a wiring regression class a call-site diff (e.g. reverting
// `swipeable-columns.tsx`'s comparator back to a hardcoded
// `compareTasksByCreatedDesc`) would not be caught by `task-order.test.ts`
// alone, since that file only tests the comparator functions in isolation,
// never that a registered view actually calls them with the live setting.
//
// The desktop "kanban" entry renders through `@dnd-kit/core` (drag sensors)
// and `@tanstack/react-virtual` (column virtualization), neither of which
// measures real layout under happy-dom. Both are stubbed here to render every
// item unconditionally, mirroring the precedented `@dnd-kit/core` mock in
// `kanban-drag-surface.test.tsx` — the stubs affect only rendering mechanics,
// never the sort/filter data path under test.
vi.mock("@dnd-kit/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dnd-kit/core")>();
  return {
    ...actual,
    DndContext: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    DragOverlay: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    useSensor: () => null,
    useSensors: () => [],
    useDroppable: () => ({ setNodeRef: () => {}, isOver: false }),
  };
});

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (opts: { count: number; getItemKey?: (index: number) => string | number }) => ({
    getTotalSize: () => opts.count * 96,
    getVirtualItems: () =>
      Array.from({ length: opts.count }, (_, index) => ({
        index,
        start: index * 96,
        key: opts.getItemKey ? opts.getItemKey(index) : index,
      })),
    measureElement: () => {},
  }),
}));

beforeEach(() => {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1280 });
});
afterEach(() => {
  cleanup();
});

const STEP: WorkflowStep = { id: "step-1", title: "In Progress", color: "bg-blue-500" };

function makeTask(id: string, priority: Task["priority"], createdAt: string): Task {
  return {
    id,
    title: id,
    workflowStepId: STEP.id,
    state: "TODO",
    priority,
    createdAt,
  } as Task;
}

// Priority order (critical first) disagrees with creation order (newest
// first) so a view that silently ignores `kanbanSort` produces a detectably
// different, wrong order for one of the two settings below.
const TASKS: Task[] = [
  makeTask("low-newest", "low", "2026-01-03T00:00:00Z"),
  makeTask("critical-oldest", "critical", "2026-01-01T00:00:00Z"),
];

const noop = () => undefined;

async function renderView(storedValue: string, kanbanSort: "created_desc" | "priority_desc") {
  const entry = getViewByStoredValue(storedValue);
  if (!entry) throw new Error(`no VIEW_REGISTRY entry for storedValue ${storedValue}`);
  const Component = entry.component;
  let result: ReturnType<typeof render>;
  await act(async () => {
    result = render(
      <StateProvider initialState={{ userSettings: { kanbanSort } } as never}>
        <ToastProvider>
          <Component
            workflowId="wf-1"
            steps={[STEP]}
            moveTargetSteps={[STEP]}
            tasks={TASKS}
            onPreviewTask={noop}
            onOpenTask={noop}
            onEditTask={noop}
            onDeleteTask={noop}
          />
        </ToastProvider>
      </StateProvider>,
    );
  });
  return result!;
}

describe("VIEW_REGISTRY ordering wiring (AC-002.5)", () => {
  it("the pipeline view (graph2) reorders tasks by priority under priority_desc, and by recency under created_desc", async () => {
    const { container: createdDescContainer } = await renderView("graph2", "created_desc");
    const createdOrder = [
      ...createdDescContainer.querySelectorAll("[data-testid^='pipeline-task-']"),
    ]
      .map((el) => el.getAttribute("data-testid"))
      .filter((id): id is string => !!id && !id.includes("-actions-") && !id.includes("-repo-"));
    expect(createdOrder).toEqual(["pipeline-task-low-newest", "pipeline-task-critical-oldest"]);

    cleanup();

    const { container: priorityDescContainer } = await renderView("graph2", "priority_desc");
    const priorityOrder = [
      ...priorityDescContainer.querySelectorAll("[data-testid^='pipeline-task-']"),
    ]
      .map((el) => el.getAttribute("data-testid"))
      .filter((id): id is string => !!id && !id.includes("-actions-") && !id.includes("-repo-"));
    expect(priorityOrder).toEqual(["pipeline-task-critical-oldest", "pipeline-task-low-newest"]);
  });

  it("the kanban board view reorders its column by priority under priority_desc, and by recency under created_desc", async () => {
    await renderView("", "created_desc");
    const column = screen.getByTestId(`kanban-column-${STEP.id}`);
    const createdTitles = within(column)
      .getAllByText(/^(low-newest|critical-oldest)$/)
      .map((el) => el.textContent);
    expect(createdTitles).toEqual(["low-newest", "critical-oldest"]);

    cleanup();

    await renderView("", "priority_desc");
    const priorityColumn = screen.getByTestId(`kanban-column-${STEP.id}`);
    const priorityTitles = within(priorityColumn)
      .getAllByText(/^(low-newest|critical-oldest)$/)
      .map((el) => el.textContent);
    expect(priorityTitles).toEqual(["critical-oldest", "low-newest"]);
  });
});
