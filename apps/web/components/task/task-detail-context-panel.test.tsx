import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

import type { TaskContextDTO } from "@/lib/api/domains/office-task-context-api";
import { TaskDetailContextPanel } from "./task-detail-context-panel";

afterEach(cleanup);

const baseContext = (overrides: Partial<TaskContextDTO> = {}): TaskContextDTO => ({
  task: {
    id: "task-1",
    identifier: "KAN-1",
    title: "Implement endpoint",
    state: "in_progress",
    workspace_id: "ws-1",
  },
  children: [],
  siblings: [],
  blockers: [],
  blocked_by: [],
  available_documents: [],
  ...overrides,
});

const parentTask: TaskRefForTest = {
  id: "parent",
  identifier: "KAN-2",
  title: "Plan",
  state: "completed",
  workspace_id: "ws-1",
};

type TaskRefForTest = {
  id: string;
  identifier: string;
  title: string;
  state: string;
  workspace_id: string;
  parent_id?: string;
};

describe("TaskDetailContextPanel", () => {
  it("renders nothing when context is null", () => {
    const { container } = render(<TaskDetailContextPanel context={null} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when context has no relations / docs / workspace", () => {
    const { container } = render(<TaskDetailContextPanel context={baseContext()} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders parent and siblings", () => {
    render(
      <TaskDetailContextPanel
        context={baseContext({
          parent: parentTask,
          siblings: [
            {
              id: "sib1",
              identifier: "KAN-3",
              title: "Spec",
              state: "in_progress",
              workspace_id: "ws-1",
              parent_id: "parent",
            },
          ],
        })}
      />,
    );
    expect(screen.getByText("Related tasks")).toBeTruthy();
    expect(screen.getByText("KAN-2 Plan")).toBeTruthy();
    expect(screen.getByText("KAN-3 Spec")).toBeTruthy();
  });

  it("renders document references with key + title but no body content", () => {
    render(
      <TaskDetailContextPanel
        context={baseContext({
          available_documents: [{ task: parentTask, key: "spec", title: "Architecture spec" }],
        })}
      />,
    );
    expect(screen.getByText("Documents available")).toBeTruthy();
    expect(screen.getByText("spec")).toBeTruthy();
    expect(screen.getByText(/Architecture spec/)).toBeTruthy();
  });

  it("renders the shared-workspace badge with member count", () => {
    render(
      <TaskDetailContextPanel
        context={baseContext({
          workspace_mode: "inherit_parent",
          workspace_group: {
            id: "g1",
            owned_by_kandev: true,
            materialized_path: "/wt/abc",
            members: [
              parentTask,
              {
                id: "task-1",
                identifier: "KAN-1",
                title: "Implement",
                state: "in_progress",
                workspace_id: "ws-1",
              },
            ],
          },
        })}
      />,
    );
    expect(screen.getByText(/Shared workspace · 2 members/)).toBeTruthy();
    expect(screen.getByTestId("workspace-mode-label").textContent).toContain("inherit_parent");
    expect(screen.getByText("/wt/abc")).toBeTruthy();
  });

  it("renders the requires-configuration banner when workspace_status flips", () => {
    render(
      <TaskDetailContextPanel
        context={baseContext({
          workspace_status: "requires_configuration",
          workspace_group: {
            id: "g1",
            owned_by_kandev: true,
            cleanup_status: "cleanup_failed",
            members: [],
          },
        })}
      />,
    );
    expect(screen.getByTestId("task-detail-context-panel-requires-config")).toBeTruthy();
  });
});
