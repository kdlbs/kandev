import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { BootRouteData } from "./boot-payload";
import { SpaRoutes } from "./spa-routes";

const consumer = vi.hoisted(() => vi.fn());
vi.mock("@/app/tasks/tasks-page-client", () => ({
  TasksPageClient: (props: unknown) => {
    consumer(props);
    return null;
  },
}));
afterEach(() => {
  cleanup();
  consumer.mockClear();
  window.history.replaceState({}, "", "/");
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
    <StateProvider>
      <SpaRoutes routeData={routeData} />
    </StateProvider>,
  );
  expect(consumer.mock.calls.at(-1)?.[0]).toMatchObject({
    initialGroup: "workflow_step",
    initialSort: "title_desc",
  });
});
