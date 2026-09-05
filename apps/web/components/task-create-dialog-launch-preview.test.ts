import { describe, expect, it } from "vitest";
import {
  composeLaunchPreviewPrompt,
  type TaskCreateLaunchIntent,
  resolveTaskCreateLaunchPreview,
} from "./task-create-dialog-launch-preview";
import type { StepType } from "./task-create-dialog-types";

const WORKFLOW_ID = "workflow-1";

function step(id: string, position: number, overrides: Partial<StepType> = {}): StepType {
  return { id, title: id, position, ...overrides };
}

function resolve(
  snapshotSteps: StepType[],
  fetchedSteps: StepType[] | null = null,
  launchIntent: TaskCreateLaunchIntent = "start-agent",
) {
  return resolveTaskCreateLaunchPreview({
    effectiveWorkflowId: WORKFLOW_ID,
    fetchedSteps,
    snapshotSteps,
    launchIntent,
  });
}

describe("task-create launch preview", () => {
  it("selects the first auto-start step by position", () => {
    expect(
      resolve([
        step("later-auto-start", 3, {
          title: "Later automated step",
          events: { on_enter: [{ type: "auto_start_agent" }] },
        }),
        step("auto-start", 1, {
          title: "In Progress",
          prompt: "Implement: {{task_prompt}}",
          events: { on_enter: [{ type: "auto_start_agent" }] },
        }),
      ]),
    ).toEqual({
      stepId: "auto-start",
      stepName: "In Progress",
      stepPrompt: "Implement: {{task_prompt}}",
    });
  });

  it("falls back to the configured start step when no step auto-starts an agent", () => {
    expect(
      resolve([
        step("first", 0, { title: "First" }),
        step("configured-start", 1, { title: "Configured start", is_start_step: true }),
      ]),
    ).toMatchObject({ stepId: "configured-start", stepName: "Configured start" });
  });

  it("falls back to the first positional step when no start step is configured", () => {
    expect(resolve([step("second", 2), step("first", 1)])).toMatchObject({
      stepId: "first",
      stepName: "first",
    });
  });

  it("uses the first positional step for the empty-description plan-mode action", () => {
    expect(
      resolve(
        [
          step("backlog", 0, { title: "Backlog" }),
          step("configured", 1, { title: "Configured", is_start_step: true }),
          step("auto-start", 2, {
            title: "In Progress",
            events: { on_enter: [{ type: "auto_start_agent" }] },
          }),
        ],
        null,
        "plan-mode",
      ),
    ).toMatchObject({ stepId: "backlog", stepName: "Backlog" });
  });

  it("uses matching fetched steps and never selects a stale workflow step", () => {
    expect(
      resolve(
        [step("snapshot-start", 0, { title: "Snapshot start" })],
        [
          step("stale", 0, {
            title: "Stale workflow",
            workflowId: "old-workflow",
            events: { on_enter: [{ type: "auto_start_agent" }] },
          }),
          step("current", 1, {
            title: "Fetched current",
            workflowId: WORKFLOW_ID,
            events: { on_enter: [{ type: "auto_start_agent" }] },
          }),
        ],
      ),
    ).toMatchObject({ stepId: "current", stepName: "Fetched current" });
  });

  it("falls back to the selected workflow snapshot when fetched steps are stale", () => {
    expect(
      resolve(
        [step("selected", 0, { title: "Selected workflow" })],
        [step("stale", 0, { workflowId: "old-workflow", title: "Stale workflow" })],
      ),
    ).toMatchObject({ stepId: "selected", stepName: "Selected workflow" });
  });

  it("treats an empty successful fetch as authoritative over the snapshot", () => {
    expect(resolve([step("stale-snapshot", 0, { title: "Stale snapshot" })], [])).toBeNull();
  });

  it("omits a destination when there is no effective workflow or no step", () => {
    expect(
      resolveTaskCreateLaunchPreview({
        effectiveWorkflowId: null,
        fetchedSteps: null,
        snapshotSteps: [],
      }),
    ).toBeNull();
    expect(resolve([], [])).toBeNull();
  });
});

describe("composeLaunchPreviewPrompt", () => {
  it("replaces only the first task prompt token", () => {
    expect(
      composeLaunchPreviewPrompt(
        "Start {{task_prompt}} then {{task_prompt}} for {task_id} and @saved",
        "Review the change",
      ),
    ).toBe("Start Review the change then {{task_prompt}} for {task_id} and @saved");
  });

  it("uses the complete step prompt when it has no task prompt token", () => {
    expect(composeLaunchPreviewPrompt("Run the saved instructions for {task_id}", "Ignored")).toBe(
      "Run the saved instructions for {task_id}",
    );
  });
});
