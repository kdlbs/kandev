import { describe, expect, it } from "vitest";

import {
  newWorkflowEditorPath,
  readWorkflowEditorSelection,
  workflowEditorSelectionPath,
} from "./workflow-editor-paths";

describe("workflow editor paths", () => {
  it("carries the selected template and draft name to the client-only route", () => {
    expect(
      newWorkflowEditorPath("workspace/one", {
        templateId: "template/one",
        name: "  Draft workflow  ",
      }),
    ).toBe(
      "/settings/workspaces/workspace%2Fone/workflows/new?template=template%2Fone&name=Draft+workflow",
    );
  });

  it("does not add empty draft query values", () => {
    expect(newWorkflowEditorPath("workspace-1")).toBe(
      "/settings/workspaces/workspace-1/workflows/new",
    );
  });

  it("parses only supported shallow editor selections", () => {
    expect(
      readWorkflowEditorSelection(
        new URLSearchParams("step=step%2Fone&tab=automation&trigger=on_enter&action=2"),
      ),
    ).toEqual({
      stepId: "step/one",
      tab: "automation",
      trigger: "on_enter",
      actionIndex: 2,
    });
    expect(readWorkflowEditorSelection(new URLSearchParams("tab=unknown&action=-1"))).toEqual({
      stepId: null,
      tab: "agent",
      trigger: null,
      actionIndex: null,
    });
  });

  it("writes selection state while preserving draft creation parameters", () => {
    expect(
      workflowEditorSelectionPath(
        "/settings/workspaces/workspace-1/workflows/new",
        new URLSearchParams("template=review&name=Draft+workflow"),
        {
          stepId: "step-1",
          tab: "automation",
          trigger: "on_enter",
          actionIndex: 0,
        },
      ),
    ).toBe(
      "/settings/workspaces/workspace-1/workflows/new?template=review&name=Draft+workflow&step=step-1&tab=automation&trigger=on_enter&action=0",
    );
  });
});
