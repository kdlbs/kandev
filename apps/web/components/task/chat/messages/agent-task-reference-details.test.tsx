import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AgentTaskReferenceDetails } from "./agent-task-reference-details";

const details = vi.hoisted(() => ({
  task: { id: "target", title: "Target", parentTaskId: "unavailable-parent-id" },
  parentTask: null as { title: string } | null,
  workflowName: null,
  createdTimestamp: null,
}));
vi.mock("@/hooks/domains/task/use-task-reference-details", () => ({
  useTaskReferenceDetails: () => details,
}));
afterEach(() => {
  cleanup();
  details.parentTask = null;
});

it("omits an unavailable parent instead of exposing its raw ID", () => {
  render(<AgentTaskReferenceDetails reference={{ taskId: "target", title: "Target" }} />);
  expect(screen.queryByText("unavailable-parent-id")).toBeNull();
  expect(screen.queryByText("Parent")).toBeNull();
});

it("shows the parent once its title is loaded", () => {
  details.parentTask = { title: "Terminal reliability" };
  render(<AgentTaskReferenceDetails reference={{ taskId: "target", title: "Target" }} />);
  expect(screen.getByText("Terminal reliability")).toBeTruthy();
});
