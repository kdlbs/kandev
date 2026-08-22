import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import { DEFAULT_TASKS_LIST_GROUP, DEFAULT_TASKS_LIST_SORT } from "@/lib/tasks/tasks-list-options";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type Task,
} from "@/lib/types/http";
import { TasksListView, type TasksListViewProps } from "./tasks-list-view";

afterEach(cleanup);

const PLUGIN_TAGS_FACET = "facet:plugin:tags";
const TASKS_LIST_SECTION = "tasks-list-section";

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

function makeTask(overrides: Partial<Task>): Task {
  return {
    id: toTaskId("task-1"),
    title: "A task",
    state: "WAITING_FOR_INPUT",
    workflow_step_id: "step-1",
    primary_session_id: toSessionId("session-1"),
    ...overrides,
  } as Task;
}

function props(tasks: Task[]): TasksListViewProps {
  return {
    total: tasks.length,
    showArchived: false,
    setShowArchived: () => undefined,
    tasksListSort: DEFAULT_TASKS_LIST_SORT,
    onTasksListSortChange: () => undefined,
    tasksListGroup: DEFAULT_TASKS_LIST_GROUP,
    onTasksListGroupChange: () => undefined,
    tasks,
    workflows: [],
    repositories: [],
    showTaskDetails: false,
    pageCount: 1,
    pagination: { pageIndex: 0, pageSize: 25 },
    setPagination: () => undefined,
    isLoading: false,
    handleRowClick: () => undefined,
    deletingTaskId: null,
    handleArchive: async () => undefined,
    handleUnarchive: async () => undefined,
    handleDelete: async () => undefined,
  };
}

function renderList(
  task: Task,
  messagesBySession: Record<string, Message[]> = {},
  showTaskDetails = false,
) {
  return render(
    <StateProvider initialState={{ messages: { bySession: messagesBySession, metaBySession: {} } }}>
      <TooltipProvider>
        <TasksListView {...props([task])} showTaskDetails={showTaskDetails} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("TasksListView row — waiting-for-input parity", () => {
  it("renders the message-question for a pending clarification (path previously disabled)", () => {
    const { container } = renderList(makeTask({}), {
      "session-1": [message({ type: "clarification_request", metadata: { status: "pending" } })],
    });
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });

  it("renders the shield-question for a pending permission, distinct from done and running", () => {
    const { container } = renderList(makeTask({}), {
      "session-1": [message({ type: "permission_request", metadata: { status: "pending" } })],
    });
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });

  it("keeps the pending permission indicator when rich rows are enabled", () => {
    const { container } = renderList(
      makeTask({}),
      { "session-1": [message({ type: "permission_request", metadata: { status: "pending" } })] },
      true,
    );
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
  });

  it("falls back to the boot snapshot pending action when messages are not loaded", () => {
    const { container } = renderList(
      makeTask({
        primary_session_state: "WAITING_FOR_INPUT",
        primary_session_pending_action: "permission",
      }),
    );
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
  });

  it("shows the plain waiting question for a finished turn awaiting a reply", () => {
    const { container } = renderList(makeTask({}), { "session-1": [] });
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
  });
});

describe("TasksListView row — destructive-action guard", () => {
  it("warns from the paginated row data even when the task is absent from kanban state", () => {
    renderList(makeTask({ foreground_activity: "background" }));

    fireEvent.click(screen.getByRole("button", { name: "Delete task" }));
    expect(screen.queryByTestId("still-working-warning")).not.toBeNull();
  });
});

describe("TasksListView facet grouping", () => {
  it("keeps a plugin value named untagged separate from the generated ungrouped section", () => {
    const tagged = makeTask({ id: toTaskId("tagged") });
    const untagged = makeTask({ id: toTaskId("untagged") });

    render(
      <StateProvider initialState={{ messages: { bySession: {}, metaBySession: {} } }}>
        <TooltipProvider>
          <TasksListView
            {...props([tagged, untagged])}
            tasksListGroup={PLUGIN_TAGS_FACET}
            facetValues={{
              "facet:plugin:tags:tagged": [{ value: "untagged", label: "Custom untagged" }],
            }}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    expect(screen.getAllByTestId(TASKS_LIST_SECTION)).toHaveLength(2);
  });

  it("keeps the host fallback separate from any plugin value", () => {
    const tagged = makeTask({ id: toTaskId("tagged") });
    const untagged = makeTask({ id: toTaskId("untagged") });

    render(
      <StateProvider initialState={{ messages: { bySession: {}, metaBySession: {} } }}>
        <TooltipProvider>
          <TasksListView
            {...props([tagged, untagged])}
            tasksListGroup={PLUGIN_TAGS_FACET}
            facetValues={{
              "facet:plugin:tags:tagged": [
                { value: "__host_ungrouped__", label: "Custom host value" },
              ],
            }}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    expect(screen.getAllByTestId(TASKS_LIST_SECTION)).toHaveLength(2);
  });

  // A wrapping section label used to squeeze the flex sibling swatch down to
  // ~1px wide, which erased the plugin's colour coding exactly when the label
  // was long enough to need it.
  it("keeps the colour swatch from shrinking when a long facet label wraps", () => {
    const tagged = makeTask({ id: toTaskId("tagged") });

    render(
      <StateProvider initialState={{ messages: { bySession: {}, metaBySession: {} } }}>
        <TooltipProvider>
          <TasksListView
            {...props([tagged])}
            tasksListGroup={PLUGIN_TAGS_FACET}
            facetValues={{
              "facet:plugin:tags:tagged": [
                { value: "long", label: "A very long facet label".repeat(6), color: "#0ea5e9" },
              ],
            }}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    const swatch = screen
      .getByTestId(TASKS_LIST_SECTION)
      .querySelector('span[style*="background-color"]');
    expect(swatch).not.toBeNull();
    expect(swatch?.classList.contains("shrink-0")).toBe(true);
  });

  it("groups a task under every facet value it carries", () => {
    const multi = makeTask({ id: toTaskId("multi") });
    const single = makeTask({ id: toTaskId("single") });

    render(
      <StateProvider initialState={{ messages: { bySession: {}, metaBySession: {} } }}>
        <TooltipProvider>
          <TasksListView
            {...props([multi, single])}
            tasksListGroup={PLUGIN_TAGS_FACET}
            facetValues={{
              "facet:plugin:tags:multi": [
                { value: "alpha", label: "Alpha" },
                { value: "beta", label: "Beta" },
              ],
              "facet:plugin:tags:single": [{ value: "beta", label: "Beta" }],
            }}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    const titles = screen
      .getAllByTestId(TASKS_LIST_SECTION)
      .map((section) => section.textContent ?? "");
    expect(titles).toHaveLength(2);
    expect(titles[0]).toContain("Alpha");
    expect(titles[0]).toContain("1");
    expect(titles[1]).toContain("Beta");
    expect(titles[1]).toContain("2");
  });
});

describe("TasksListView row — task-row-metadata slot", () => {
  const PLUGIN_ID = "example-metadata-plugin";
  const METADATA_TEST_ID = "plugin-row-metadata";
  const SLOT = "task-row-metadata";

  function SlotPropsProbe({ slotProps }: { slotProps?: unknown }) {
    const p = slotProps as {
      taskId: string;
      workspaceId: string | null;
      workflowStepId: string | null;
      surface: string;
    };
    return (
      <span data-testid={METADATA_TEST_ID}>
        {p.taskId}|{p.workspaceId}|{p.workflowStepId}|{p.surface}
      </span>
    );
  }

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  function renderWithWorkspace(showTaskDetails: boolean) {
    return render(
      <StateProvider initialState={{ workspaces: { items: [], activeId: "ws-1" } }}>
        <TooltipProvider>
          <TasksListView {...props([makeTask({})])} showTaskDetails={showTaskDetails} />
        </TooltipProvider>
      </StateProvider>,
    );
  }

  it("renders the slot with surface: 'task-list' in the rich row variant", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerComponent(SLOT, SlotPropsProbe);

    renderWithWorkspace(true);

    expect(screen.getByTestId(METADATA_TEST_ID).textContent).toBe("task-1|ws-1|step-1|task-list");
  });

  it("renders the slot with surface: 'task-list' in the compact row variant", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerComponent(SLOT, SlotPropsProbe);

    renderWithWorkspace(false);

    expect(screen.getByTestId(METADATA_TEST_ID).textContent).toBe("task-1|ws-1|step-1|task-list");
  });

  it("renders nothing in either variant when no plugin is registered", () => {
    renderWithWorkspace(true);
    expect(screen.queryByTestId(METADATA_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-row-metadata")).toBeNull();
    cleanup();

    renderWithWorkspace(false);
    expect(screen.queryByTestId(METADATA_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("task-row-metadata")).toBeNull();
  });
});
