import { describe, expect, it } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import { buildWorkflowAgentOverrideValidation } from "./task-create-dialog-workflow-agent-override-validation";

const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = "workspace-1";

const profiles = [
  {
    id: "source",
    label: "Source",
    agent_id: "agent-source",
    agent_name: "source",
    cli_passthrough: false,
    enabled: true,
  },
  {
    id: "replacement",
    label: "Replacement",
    agent_id: "agent-replacement",
    agent_name: "replacement",
    cli_passthrough: false,
    enabled: true,
  },
] satisfies AgentProfileOption[];

const base = {
  effectiveWorkflowId: WORKFLOW_ID,
  workspaceId: WORKSPACE_ID,
  profiles,
  replacementOptions: profiles.map((profile) => ({
    value: profile.id,
    label: profile.label,
    renderLabel: () => profile.label,
  })),
  overrides: { source: "replacement" },
  isCreateMode: true,
};

describe("workflow agent override submit validation", () => {
  it("blocks create while the workflow snapshot is loading or failed", () => {
    const loading = buildWorkflowAgentOverrideValidation({
      ...base,
      snapshots: {
        [WORKFLOW_ID]: {
          workflowId: WORKFLOW_ID,
          workflowName: "Workflow",
          steps: [],
          tasks: [],
          isPlaceholder: true,
        },
      },
      workspaceSnapshotRead: { workspaceId: WORKSPACE_ID, error: false },
    });
    expect(loading.blockedReason).toBeTruthy();

    const failed = buildWorkflowAgentOverrideValidation({
      ...base,
      snapshots: {},
      workspaceSnapshotRead: { workspaceId: WORKSPACE_ID, error: true },
    });
    expect(failed.blockedReason).toBeTruthy();
  });

  it("does not block ordinary create while an unused workflow snapshot is loading or failed", () => {
    const ordinary = { ...base, overrides: {} };
    const loading = buildWorkflowAgentOverrideValidation({
      ...ordinary,
      snapshots: {
        [WORKFLOW_ID]: {
          workflowId: WORKFLOW_ID,
          workflowName: "Workflow",
          steps: [],
          tasks: [],
          isPlaceholder: true,
        },
      },
      workspaceSnapshotRead: { workspaceId: WORKSPACE_ID, error: false },
    });
    const failed = buildWorkflowAgentOverrideValidation({
      ...ordinary,
      snapshots: {},
      workspaceSnapshotRead: { workspaceId: WORKSPACE_ID, error: true },
    });
    expect(loading.blockedReason).toBeUndefined();
    expect(failed.blockedReason).toBeUndefined();
  });

  it("blocks an unavailable replacement in create mode and leaves edit mode unblocked", () => {
    const args = {
      ...base,
      snapshots: {
        [WORKFLOW_ID]: {
          workflowId: WORKFLOW_ID,
          workflowName: "Workflow",
          steps: [
            {
              id: "implement",
              title: "Implement",
              color: "blue",
              position: 1,
              agent_profile_id: "source",
            },
          ],
          tasks: [],
        },
      },
      workspaceSnapshotRead: { workspaceId: WORKSPACE_ID, error: false },
      replacementOptions: base.replacementOptions.filter(
        (option) => option.value !== "replacement",
      ),
    };
    expect(buildWorkflowAgentOverrideValidation(args).blockedReason).toBeTruthy();
    expect(
      buildWorkflowAgentOverrideValidation({ ...args, isCreateMode: false }).blockedReason,
    ).toBeUndefined();
  });
});
