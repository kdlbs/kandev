import { describe, expect, it } from "vitest";
import { selectMobileNavigatorWorkflows, selectWorkflowSwimlanes } from "./workflow-swimlanes";

const IMPROVE_WORKFLOW_NAME = "Improve Kandev";

describe("selectWorkflowSwimlanes — hidden workflow filter resolution", () => {
  const workflows = [
    { id: "dev", name: "Development", hidden: false },
    { id: "improve", name: IMPROVE_WORKFLOW_NAME, hidden: true },
  ];
  const snapshots = {
    dev: { workflowName: "Development" },
    improve: { workflowName: IMPROVE_WORKFLOW_NAME },
  };

  it("keeps hidden workflows off the All Workflows board", () => {
    expect(selectWorkflowSwimlanes(null, workflows, snapshots).map((w) => w.id)).toEqual(["dev"]);
  });

  it("renders a hidden workflow the user explicitly selects", () => {
    expect(selectWorkflowSwimlanes("improve", workflows, snapshots).map((w) => w.id)).toEqual([
      "improve",
    ]);
  });

  it("does not render a hidden workflow before its snapshot has loaded", () => {
    expect(selectWorkflowSwimlanes("improve", workflows, {})).toEqual([]);
  });

  it("still renders a visible workflow under an explicit filter", () => {
    expect(selectWorkflowSwimlanes("dev", workflows, snapshots).map((w) => w.id)).toEqual(["dev"]);
  });

  it("returns no workflows for a filter that does not exist in the store", () => {
    expect(selectWorkflowSwimlanes("missing", workflows, snapshots)).toEqual([]);
  });
});

describe("selectMobileNavigatorWorkflows — mobile board navigator options", () => {
  const workflows = [
    { id: "dev", name: "Development", hidden: false },
    { id: "improve", name: IMPROVE_WORKFLOW_NAME, hidden: true },
    { id: "report", name: "Report Kandev Issue", hidden: true },
  ];
  const visibleOrdered = [{ id: "dev", name: "Development", hidden: false }];
  const noTasks = () => [];
  const tasks = (workflowId: string) => (workflowId === "improve" ? [{ id: "t1" }] : []);

  it("lists visible workflows plus hidden workflows that have tasks", () => {
    const entries = selectMobileNavigatorWorkflows(visibleOrdered, workflows, tasks);
    expect(entries.map((entry) => entry.workflow.id)).toEqual(["dev", "improve"]);
  });

  it("keeps empty hidden workflows out of the navigator", () => {
    expect(
      selectMobileNavigatorWorkflows(visibleOrdered, workflows, noTasks).map(
        (entry) => entry.workflow.id,
      ),
    ).toEqual(["dev"]);
  });

  it("returns the filtered tasks alongside each workflow so callers reuse the result", () => {
    const entries = selectMobileNavigatorWorkflows(visibleOrdered, workflows, tasks);
    expect(entries.find((entry) => entry.workflow.id === "improve")?.tasks).toEqual([{ id: "t1" }]);
  });

  // `both` marks the hidden improve workflow as already visible. `withTasks`
  // gives every workflow tasks, so the hidden `report` workflow from the
  // module-scope fixture is added too — the point is that improve is not
  // duplicated just because it is both hidden and already in the visible list.
  it("does not duplicate a hidden workflow already in the visible list but includes other hidden workflows with tasks", () => {
    const both = [
      { id: "dev", name: "Development", hidden: false },
      { id: "improve", name: IMPROVE_WORKFLOW_NAME, hidden: true },
    ];
    const withTasks = () => [{ id: "t1" }];
    expect(
      selectMobileNavigatorWorkflows(both, workflows, withTasks).map((entry) => entry.workflow.id),
    ).toEqual(["dev", "improve", "report"]);
  });
});
