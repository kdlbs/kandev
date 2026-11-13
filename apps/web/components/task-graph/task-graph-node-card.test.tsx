import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { TaskGraphNode } from "@/lib/task-graph/model";
import { TaskGraphNodeCard } from "./task-graph-node-card";
import { TaskGraphLegend } from "./task-graph-legend";

afterEach(cleanup);

function node(overrides: Partial<TaskGraphNode> = {}): TaskGraphNode {
  return {
    id: "t1",
    title: "Write API",
    state: "IN_PROGRESS",
    depth: 1,
    role: "member",
    archived: false,
    blocked: false,
    step: 2,
    focused: false,
    ...overrides,
  };
}

describe("TaskGraphNodeCard", () => {
  it("shows step, title and state", () => {
    render(
      <TaskGraphNodeCard
        node={node()}
        state={{ dimmed: false, emphasized: false, selected: false }}
      />,
    );
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getByText("Write API")).toBeTruthy();
    expect(screen.getByText("In progress")).toBeTruthy();
  });

  it("labels blocked, failed, focus, external, archived and nested parent", () => {
    const { rerender } = render(
      <TaskGraphNodeCard
        node={node({
          blocked: true,
          blocked_reason: "pending",
          focused: true,
          depth: 2,
          parent_title: "Backend",
        })}
        state={{ dimmed: false, emphasized: false, selected: false }}
      />,
    );
    expect(screen.getByText("Blocked")).toBeTruthy();
    expect(screen.getByText("This task")).toBeTruthy();
    expect(screen.getByText("in Backend")).toBeTruthy();
    expect(screen.getByTestId("task-graph-node-card").dataset.focused).toBe("true");

    rerender(
      <TaskGraphNodeCard
        node={node({ blocked: true, blocked_reason: "failed", role: "external", archived: true })}
        state={{ dimmed: true, emphasized: false, selected: false }}
      />,
    );
    expect(screen.getByText("Predecessor failed")).toBeTruthy();
    expect(screen.getByText("Outside this task")).toBeTruthy();
    expect(screen.getByText("Archived")).toBeTruthy();
    expect(screen.getByTestId("task-graph-node-card").dataset.dimmed).toBe("true");
  });

  it("marks the selected node", () => {
    render(
      <TaskGraphNodeCard
        node={node()}
        state={{ dimmed: false, emphasized: false, selected: true }}
      />,
    );
    expect(screen.getByTestId("task-graph-node-card").dataset.selected).toBe("true");
  });

  it("does not name the parent of a direct child", () => {
    render(
      <TaskGraphNodeCard
        node={node({ parent_title: "Root" })}
        state={{ dimmed: false, emphasized: false, selected: false }}
      />,
    );
    expect(screen.queryByText("in Root")).toBeNull();
  });
});

describe("TaskGraphLegend", () => {
  it("names each edge style in text", () => {
    render(<TaskGraphLegend showExternal />);
    for (const label of ["Done", "Waiting", "Failed", "Related", "Task outside the subtree"]) {
      expect(screen.getByText(label)).toBeTruthy();
    }
  });

  it("omits the external entry when no external nodes exist", () => {
    render(<TaskGraphLegend showExternal={false} />);
    expect(screen.queryByText("Task outside the subtree")).toBeNull();
  });
});
