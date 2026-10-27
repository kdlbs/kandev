import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";

const controls = vi.hoisted(() => ({
  isMobile: false,
  setView: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: controls.isMobile }),
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ setView: controls.setView }),
}));
vi.mock("./tasks-page-desktop-client", () => ({
  TasksPageDesktopClient: () => <div data-testid="desktop-list-client" />,
}));
vi.mock("./mobile-tasks-page-client", () => ({
  MobileTasksPageClient: ({ active }: { active: boolean }) => (
    <div data-testid="phone-list-client" data-active={String(active)} />
  ),
}));

import { TasksPageClient } from "./tasks-page-client";

afterEach(() => {
  cleanup();
  controls.isMobile = false;
  controls.setView.mockClear();
});

describe("TasksPageClient responsive composition", () => {
  it("activates the phone query consumer at the mobile breakpoint", () => {
    controls.isMobile = true;
    render(
      <StateProvider>
        <TasksPageClient
          workspaces={[]}
          initialRepositories={[]}
          initialTasks={[]}
          initialTotal={0}
          initialSort="updated_desc"
          initialGroup="none"
        />
      </StateProvider>,
    );

    expect(screen.getByTestId("phone-list-client")).toBeTruthy();
    expect(screen.getByTestId("phone-list-client").getAttribute("data-active")).toBe("true");
    expect(screen.queryByTestId("desktop-list-client")).toBeNull();
    expect(controls.setView).toHaveBeenCalledWith("list");
  });

  it("keeps the phone dialog host mounted but inactive above the mobile breakpoint", () => {
    render(
      <StateProvider>
        <TasksPageClient
          workspaces={[]}
          initialRepositories={[]}
          initialTasks={[]}
          initialTotal={0}
          initialSort="updated_desc"
          initialGroup="none"
        />
      </StateProvider>,
    );

    expect(screen.getByTestId("desktop-list-client")).toBeTruthy();
    expect(screen.getByTestId("phone-list-client").getAttribute("data-active")).toBe("false");
  });
});
