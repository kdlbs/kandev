import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const state = {
  workspaces: { activeId: "ws-1" as string | null },
  kanban: { workflowId: "wf-1" as string | null, steps: [{ id: "s1", title: "Todo" }] },
};
let officeEnabled = false;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/",
}));
vi.mock("@/app/office/components/new-task-dialog", () => ({
  NewTaskDialog: () => <div data-testid="office-new-task-dialog" />,
}));
vi.mock("@/components/task-create-dialog", () => ({
  TaskCreateDialog: () => <div data-testid="regular-task-create-dialog" />,
}));

import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";

describe("AppSidebarNewTaskItem", () => {
  beforeEach(() => {
    state.workspaces.activeId = "ws-1";
    state.kanban.workflowId = "wf-1";
    state.kanban.steps = [{ id: "s1", title: "Todo" }];
    officeEnabled = false;
  });

  afterEach(() => cleanup());

  it("uses the regular task-create dialog when office is disabled", () => {
    officeEnabled = false;
    render(<AppSidebarNewTaskItem collapsed={false} />);
    expect(screen.getByTestId("regular-task-create-dialog")).toBeTruthy();
    expect(screen.queryByTestId("office-new-task-dialog")).toBeNull();
  });

  it("uses the office new-issue dialog when office is enabled", () => {
    officeEnabled = true;
    render(<AppSidebarNewTaskItem collapsed={false} />);
    expect(screen.getByTestId("office-new-task-dialog")).toBeTruthy();
    expect(screen.queryByTestId("regular-task-create-dialog")).toBeNull();
  });

  it("renders no dialog when there is no active workspace", () => {
    state.workspaces.activeId = null;
    render(<AppSidebarNewTaskItem collapsed={false} />);
    expect(screen.queryByTestId("regular-task-create-dialog")).toBeNull();
    expect(screen.queryByTestId("office-new-task-dialog")).toBeNull();
  });
});
