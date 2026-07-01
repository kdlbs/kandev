import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, fireEvent } from "@testing-library/react";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { qk } from "@/lib/query/keys";
import {
  taskId,
  workflowId,
  workspaceId,
  type Task,
  type Workflow,
  type WorkflowSnapshot,
} from "@/lib/types/http";
import {
  KanbanCardDropdownMenuItems,
  useKanbanCardMoveTargets,
  type KanbanCardMenuEntry,
} from "./kanban-card-menu-items";

const WORKSPACE_ID = workspaceId("workspace-1");
const CURRENT_WORKFLOW_ID = workflowId("workflow-current");
const TARGET_WORKFLOW_ID = workflowId("workflow-target");
const TASK_ID = taskId("task-1");
const TIMESTAMP = "2026-01-01T00:00:00Z";

function queryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
      },
    },
  });
}

function wrapperFor(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

function workflow(id: string, name: string): Workflow {
  return {
    id: workflowId(id),
    workspace_id: WORKSPACE_ID,
    name,
    sort_order: 0,
    hidden: false,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function task(id: string, workflow: Workflow, stepId: string): Task {
  return {
    id: taskId(id),
    workspace_id: WORKSPACE_ID,
    workflow_id: workflow.id,
    workflow_step_id: stepId,
    position: 1,
    title: "Cached task",
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function snapshot(
  workflowItem: Workflow,
  steps: Array<{ id: string; name: string; position: number; autoStart?: boolean }>,
  tasks: Task[],
): WorkflowSnapshot {
  return {
    workflow: workflowItem,
    steps: steps.map((step) => ({
      id: step.id,
      workflow_id: workflowItem.id,
      name: step.name,
      position: step.position,
      color: "bg-sky-500",
      events: step.autoStart ? { on_enter: [{ type: "start_agent" }] } : undefined,
      allow_manual_move: true,
      created_at: TIMESTAMP,
      updated_at: TIMESTAMP,
    })),
    tasks,
  } as WorkflowSnapshot;
}

afterEach(() => {
  vi.clearAllMocks();
});

// Regression: React synthetic events bubble through the fiber tree from a Radix portal; without stopPropagation the parent Card's onClick fires instead of the confirm dialog.
describe("KanbanCardDropdownMenuItems — click propagation", () => {
  function renderWithParent(entries: KanbanCardMenuEntry[], parentOnClick: () => void) {
    return render(
      <div data-testid="parent-card" onClick={parentOnClick}>
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger>open</DropdownMenuTrigger>
          <DropdownMenuContent>
            <KanbanCardDropdownMenuItems entries={entries} />
          </DropdownMenuContent>
        </DropdownMenu>
      </div>,
    );
  }

  it("clicking a menu item does not call the parent card's onClick", () => {
    const onDelete = vi.fn();
    const parentOnClick = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      {
        kind: "item",
        key: "delete",
        label: "Delete",
        onSelect: onDelete,
      },
    ];

    renderWithParent(entries, parentOnClick);

    const deleteItem = screen.getByRole("menuitem", { name: /delete/i });
    fireEvent.click(deleteItem);

    expect(onDelete).toHaveBeenCalledTimes(1);
    expect(parentOnClick).not.toHaveBeenCalled();
  });

  it("clicking an archive menu item does not call the parent card's onClick", () => {
    const onArchive = vi.fn();
    const parentOnClick = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      {
        kind: "item",
        key: "archive",
        label: "Archive",
        onSelect: onArchive,
      },
    ];

    renderWithParent(entries, parentOnClick);

    fireEvent.click(screen.getByRole("menuitem", { name: /archive/i }));

    expect(onArchive).toHaveBeenCalledTimes(1);
    expect(parentOnClick).not.toHaveBeenCalled();
  });

  it("pointer-down on a menu item does not reach the parent (dnd-kit guard)", () => {
    const parentOnPointerDown = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      { kind: "item", key: "delete", label: "Delete", onSelect: vi.fn() },
    ];

    render(
      <div data-testid="parent-card" onPointerDown={parentOnPointerDown}>
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger>open</DropdownMenuTrigger>
          <DropdownMenuContent>
            <KanbanCardDropdownMenuItems entries={entries} />
          </DropdownMenuContent>
        </DropdownMenu>
      </div>,
    );

    fireEvent.pointerDown(screen.getByRole("menuitem", { name: /delete/i }));

    expect(parentOnPointerDown).not.toHaveBeenCalled();
  });
});

describe("useKanbanCardMoveTargets", () => {
  it("builds workflow and step move targets from Query snapshots without a legacy kanban store", () => {
    const client = queryClient();
    const currentWorkflow = workflow(CURRENT_WORKFLOW_ID, "Current");
    const targetWorkflow = workflow(TARGET_WORKFLOW_ID, "Target");
    client.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [
      currentWorkflow,
      targetWorkflow,
    ]);
    client.setQueryData(
      qk.workflows.snapshot(CURRENT_WORKFLOW_ID),
      snapshot(
        currentWorkflow,
        [
          { id: "current-1", name: "Todo", position: 0 },
          { id: "current-2", name: "Review", position: 1, autoStart: true },
        ],
        [task(TASK_ID, currentWorkflow, "current-1")],
      ),
    );
    client.setQueryData(
      qk.workflows.snapshot(TARGET_WORKFLOW_ID),
      snapshot(targetWorkflow, [{ id: "target-1", name: "Target Step", position: 0 }], []),
    );

    const { result } = renderHook(() => useKanbanCardMoveTargets(TASK_ID), {
      wrapper: wrapperFor(client),
    });

    expect(result.current.currentWorkflowId).toBe(CURRENT_WORKFLOW_ID);
    expect(result.current.workflowItems.map((item) => item.id)).toEqual([
      CURRENT_WORKFLOW_ID,
      TARGET_WORKFLOW_ID,
    ]);
    expect(result.current.stepsByWorkflowId[CURRENT_WORKFLOW_ID]).toMatchObject([
      { id: "current-1", title: "Todo" },
      { id: "current-2", title: "Review", events: { on_enter: [{ type: "start_agent" }] } },
    ]);
    expect(result.current.stepsByWorkflowId[TARGET_WORKFLOW_ID]).toMatchObject([
      { id: "target-1", title: "Target Step" },
    ]);
  });
});
