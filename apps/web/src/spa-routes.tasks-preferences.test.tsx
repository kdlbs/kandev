import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useTaskListFacets } from "@/hooks/use-task-list-facets";
import { useTaskListFacetSelection } from "@/hooks/use-task-list-facet-selection";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { Task } from "@/lib/types/http";
import type {
  TasksListGroupPreference,
  TasksListSortPreference,
} from "@/lib/tasks/tasks-list-options";
import type { BootRouteData } from "./boot-payload";
import { SpaRoutes } from "./spa-routes";

const consumer = vi.hoisted(() => vi.fn());
type TasksRouteProps = {
  initialWorkspaceId?: string;
  initialTasks: Task[];
  initialSort: TasksListSortPreference;
  initialGroup: TasksListGroupPreference;
};
vi.mock("@/app/tasks/tasks-page-client", () => ({
  TasksPageClient: (props: TasksRouteProps) => {
    const { facets, values } = useTaskListFacets(
      props.initialTasks,
      props.initialWorkspaceId ?? null,
    );
    const { displayedTasks } = useTaskListFacetSelection({
      facetKeys: facets.map((facet) => facet.key),
      requestedSort: props.initialSort,
      requestedGroup: props.initialGroup,
      tasks: props.initialTasks,
      facetValues: values,
      onSortChange: vi.fn(),
      onGroupChange: vi.fn(),
    });
    consumer({ ...props, displayedTasks });
    return null;
  },
}));
afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin("route-boot-proof");
  consumer.mockClear();
  window.history.replaceState({}, "", "/");
});

const facetKey = "facet:route-boot-proof:tags";

function renderTaskBoot(sort: string, query = "") {
  const timestamp = "2026-01-01T00:00:00Z";
  // SQLite NOCASE orders z before é; the browser's base collation reverses them.
  const tasks = [
    { id: "z", title: "z", created_at: timestamp, updated_at: timestamp },
    { id: "accent", title: "é", created_at: timestamp, updated_at: timestamp },
  ] as Task[];
  window.history.replaceState({}, "", `/tasks${query}`);
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId: "ws" } }}>
      <SpaRoutes
        routeData={
          {
            tasksPage: {
              activeWorkspaceId: "ws",
              workflows: [],
              repositories: [],
              tasks,
              total: tasks.length,
              tasksListSort: sort,
              tasksListGroup: "none",
            },
          } as BootRouteData
        }
      />
    </StateProvider>,
  );
  return { tasks, received: consumer.mock.calls.at(-1)?.[0] };
}

// @covers AC-PLUGINS-TASKLIST-FACETS-002.6
it.each(["boot preference", "query preference"])(
  "preserves boot row order for equal facet labels from the %s",
  (source) => {
    pluginRegistry.forPlugin("route-boot-proof").registerTaskListFacet({
      id: "tags",
      label: "Tag",
      getValues: () => [{ value: "same", label: "Same" }],
    });
    const { tasks, received } = renderTaskBoot(
      source === "boot preference" ? facetKey : "title_asc",
      source === "query preference" ? `?sort=${facetKey}` : "",
    );

    expect(received.initialSort).toBe(facetKey);
    expect(received.displayedTasks.map((task: Task) => task.id)).toEqual(["z", "accent"]);
    expect(received.initialTasks).not.toBe(tasks);
    expect(tasks.map((task) => task.id)).toEqual(["z", "accent"]);
  },
);

it("keeps the built-in boot sort for the same tied Unicode rows", () => {
  const { tasks, received } = renderTaskBoot("title_asc");

  expect(received.initialSort).toBe("title_asc");
  expect(received.displayedTasks.map((task: Task) => task.id)).toEqual(["accent", "z"]);
  expect(tasks.map((task) => task.id)).toEqual(["z", "accent"]);
});

// @covers AC-UI-LIST-STEP-GROUPING-001.5
it.each(["state", "workflow_step", "invalid"])("resolves a live SPA %s grouping link", (group) => {
  window.history.replaceState({}, "", `/tasks?group=${group}&sort=title_desc`);
  const routeData = {
    tasksPage: {
      activeWorkspaceId: "ws",
      workflows: [],
      repositories: [],
      tasks: [],
      total: 0,
      tasksListSort: "updated_desc",
      tasksListGroup: "state",
    },
  } as BootRouteData;
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId: "ws" } }}>
      <SpaRoutes routeData={routeData} />
    </StateProvider>,
  );
  expect(consumer.mock.calls.at(-1)?.[0]).toMatchObject({
    initialGroup: "workflow_step",
    initialSort: "title_desc",
  });
});

// @covers AC-TASKS-LIST-ARCHIVE-FILTERS-004.2
it("does not present saved active boot rows beneath an explicit archive query", () => {
  window.history.replaceState(
    {},
    "",
    "/tasks?workspace=ws&archive=archived&workflow=all&repository=all",
  );
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId: "ws" } }}>
      <SpaRoutes
        routeData={
          {
            tasksPage: {
              activeWorkspaceId: "ws",
              workflows: [],
              repositories: [],
              tasks: [{ id: "active", title: "Active boot task" } as Task],
              total: 1,
            },
          } as BootRouteData
        }
      />
    </StateProvider>,
  );
  expect(consumer.mock.calls.at(-1)?.[0]).toMatchObject({
    initialTasks: [],
    initialTotal: 0,
    initialDataLoaded: false,
  });
});
