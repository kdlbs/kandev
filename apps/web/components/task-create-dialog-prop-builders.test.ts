import { describe, expect, it } from "vitest";
import type { DialogFormBodyProps, DialogFormState, StepType } from "./task-create-dialog-types";
import {
  computeHasAllBranches,
  localRepositoryCreationEnabled,
  resolveDialogLaunchPreview,
  resolveWorkflowName,
} from "./task-create-dialog-prop-builders";

const WORKFLOW_ID = "workflow-1";
const IN_PROGRESS = "In Progress";

function formState(overrides: Partial<DialogFormState>): DialogFormState {
  return {
    noRepository: false,
    useRemote: false,
    repositories: [],
    remoteRepos: [],
    ...overrides,
  } as DialogFormState;
}

describe("computeHasAllBranches", () => {
  it("accepts no-repository tasks", () => {
    expect(computeHasAllBranches(formState({ noRepository: true }))).toBe(true);
  });

  it("requires a branch on every populated remote row", () => {
    expect(
      computeHasAllBranches(
        formState({
          useRemote: true,
          remoteRepos: [
            { key: "one", url: "https://example.com/one.git", branch: "main", source: "paste" },
            { key: "two", url: "https://example.com/two.git", branch: "", source: "paste" },
          ],
        }),
      ),
    ).toBe(false);
  });

  it("rejects remote mode without a populated row", () => {
    expect(computeHasAllBranches(formState({ useRemote: true }))).toBe(false);
  });

  it("accepts a branched remote row and ignores an empty trailing row", () => {
    expect(
      computeHasAllBranches(
        formState({
          useRemote: true,
          remoteRepos: [
            { key: "one", url: "https://example.com/one.git", branch: "main", source: "paste" },
            { key: "two", url: "", branch: "", source: "paste" },
          ],
        }),
      ),
    ).toBe(true);
  });

  it("rejects local mode without a repository row", () => {
    expect(computeHasAllBranches(formState({}))).toBe(false);
  });

  it("rejects a local repository row without a branch", () => {
    expect(
      computeHasAllBranches(
        formState({ repositories: [{ key: "one", repositoryId: "repo-one", branch: "" }] }),
      ),
    ).toBe(false);
  });

  it("requires a branch on every local repository row", () => {
    expect(
      computeHasAllBranches(
        formState({
          repositories: [
            { key: "one", repositoryId: "repo-one", branch: "main" },
            { key: "two", repositoryId: "repo-two", branch: "develop" },
          ],
        }),
      ),
    ).toBe(true);
  });
});

describe("localRepositoryCreationEnabled", () => {
  it("allows repository creation only for an unlocked new-task form", () => {
    expect(localRepositoryCreationEnabled(true, false)).toBe(true);
    expect(localRepositoryCreationEnabled(false, false)).toBe(false);
    expect(localRepositoryCreationEnabled(true, true)).toBe(false);
  });
});

describe("resolveDialogLaunchPreview", () => {
  it("projects the selected workflow snapshot only for create mode", () => {
    const steps: StepType[] = [
      {
        id: "step-1",
        title: IN_PROGRESS,
        position: 0,
        is_start_step: true,
        prompt: "Run {{task_prompt}}",
      },
    ];
    const snapshots = {
      [WORKFLOW_ID]: { steps },
    } as unknown as DialogFormBodyProps["snapshots"];

    expect(resolveDialogLaunchPreview(true, WORKFLOW_ID, null, snapshots, true)).toEqual({
      stepId: "step-1",
      stepName: IN_PROGRESS,
      stepPrompt: "Run {{task_prompt}}",
    });
    expect(resolveDialogLaunchPreview(false, WORKFLOW_ID, null, snapshots, true)).toBeNull();
  });

  it("projects the first positional step for the empty-description plan-mode action", () => {
    const steps: StepType[] = [
      { id: "backlog", title: "Backlog", position: 0, prompt: "Queue it" },
      {
        id: "auto-start",
        title: IN_PROGRESS,
        position: 1,
        events: { on_enter: [{ type: "auto_start_agent" }] },
        prompt: "Run {{task_prompt}}",
      },
    ];
    const snapshots = {
      [WORKFLOW_ID]: { steps },
    } as unknown as DialogFormBodyProps["snapshots"];

    expect(resolveDialogLaunchPreview(true, WORKFLOW_ID, null, snapshots, false)).toMatchObject({
      stepId: "backlog",
      stepName: "Backlog",
    });
    expect(resolveDialogLaunchPreview(true, WORKFLOW_ID, null, snapshots, true)).toMatchObject({
      stepId: "auto-start",
      stepName: IN_PROGRESS,
    });
  });
});

describe("resolveWorkflowName", () => {
  const workflows = [
    { id: "wf-dev", name: "Development" },
    { id: "wf-ops", name: "Operations" },
  ];

  it("returns the name of the effective workflow", () => {
    expect(resolveWorkflowName(workflows, "wf-dev")).toBe("Development");
  });

  it("returns null when no workflow is effective or the id is unknown", () => {
    expect(resolveWorkflowName(workflows, null)).toBeNull();
    expect(resolveWorkflowName(workflows, "wf-missing")).toBeNull();
  });
});
