import { createRef } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  CreateEditSelectors,
  DialogPromptSection,
  WorkflowSection,
} from "./task-create-dialog-form-body";
import type { DialogFormState, TaskFormInputsHandle } from "./task-create-dialog-types";

afterEach(cleanup);

vi.mock("@/components/workflow-selector-row", () => ({
  WorkflowSelectorRow: ({ selectedWorkflowId }: { selectedWorkflowId: string | null }) => (
    <button type="button">Workflow selector {selectedWorkflowId ?? "none"}</button>
  ),
}));

vi.mock("@/components/task-create-dialog-dependencies", () => ({
  TaskCreateDependencies: ({ value }: { value: string[] }) => (
    <button type="button" data-testid="task-create-dependencies-trigger">
      {value.length > 0 ? `${value.length} dependencies` : "No dependency"}
    </button>
  ),
}));

// Capture every render of TaskFormInputs so passthrough-related assertions can
// inspect the props that DialogPromptSection forwards to the textarea.
const taskFormInputsCalls: Array<Record<string, unknown>> = [];
vi.mock("@/components/task-create-dialog-selectors", () => ({
  TaskFormInputs: (props: Record<string, unknown>) => {
    taskFormInputsCalls.push(props);
    return (
      <textarea
        data-testid="task-description-textarea"
        placeholder={(props.placeholder as string | undefined) ?? "default placeholder"}
        disabled={Boolean(props.disabled)}
      />
    );
  },
}));

const workflow = { id: "wf-1", name: "Development" };

function renderWorkflowSection(effectiveWorkflowId: string | null) {
  return render(
    <WorkflowSection
      isCreateMode={true}
      isTaskStarted={false}
      workflows={[workflow]}
      snapshots={{}}
      effectiveWorkflowId={effectiveWorkflowId}
      onWorkflowChange={() => {}}
      agentProfiles={[]}
    />,
  );
}

describe("WorkflowSection workflow rendering", () => {
  it("keeps the selector reachable when no effective workflow is selected", () => {
    renderWorkflowSection(null);

    expect(screen.getByRole("button", { name: /workflow selector none/i })).toBeTruthy();
  });

  it("does not show redundant selector for a selected single workflow without overrides", () => {
    renderWorkflowSection("wf-1");

    expect(screen.queryByRole("button", { name: /workflow selector wf-1/i })).toBeNull();
    expect(screen.queryByTestId("task-create-dependencies-trigger")).toBeNull();
  });
});

describe("WorkflowSection", () => {
  const secondWorkflow = { id: "wf-2", name: "Support" };

  function renderWorkflowSection(
    effectiveWorkflowId: string | null,
    workflows = [workflow, secondWorkflow],
  ) {
    return render(
      <WorkflowSection
        isCreateMode
        isTaskStarted={false}
        workflows={workflows}
        snapshots={{}}
        effectiveWorkflowId={effectiveWorkflowId}
        onWorkflowChange={() => {}}
        agentProfiles={[]}
      />,
    );
  }

  it("renders the workflow selector without an inline dependency slot", () => {
    renderWorkflowSection("wf-1");

    expect(screen.getByRole("button", { name: /workflow selector wf-1/i })).toBeTruthy();
    expect(screen.queryByTestId("task-create-dependencies-trigger")).toBeNull();
  });

  it("does not render a workflow row for a single workflow without overrides", () => {
    renderWorkflowSection("wf-1", [workflow]);

    expect(screen.queryByRole("button", { name: /workflow selector wf-1/i })).toBeNull();
    expect(screen.queryByTestId("task-create-dependencies-trigger")).toBeNull();
  });

  it("keeps the workflow section independent from advanced dependencies", () => {
    renderWorkflowSection("wf-1");

    expect(screen.getByRole("button", { name: /workflow selector wf-1/i })).toBeTruthy();
    expect(screen.queryByTestId("task-create-dependency-slot")).toBeNull();
  });
});

function makeFs(): DialogFormState {
  return {
    blockedBy: [],
    setBlockedBy: () => undefined,
    mcpServerIds: [],
    setMcpServerIds: () => undefined,
    mcpServerIdsDirty: false,
    setMcpServerIdsDirty: () => undefined,
    taskName: "",
    autopilot: false,
    setAutopilot: () => {},
    setTaskName: () => {},
    hasTitle: false,
    setHasTitle: () => {},
    hasDescription: false,
    setHasDescription: () => {},
    hasPendingAttachmentUploads: false,
    setHasPendingAttachmentUploads: () => {},
    draftDescription: "",
    openCycle: 0,
    currentDefaults: { name: "", description: "" },
    descriptionInputRef: createRef<TaskFormInputsHandle>(),
    repositories: [],
    repositoriesDirty: false,
    setRepositories: () => {},
    setRepositoriesDirty: () => {},
    addRepository: () => {},
    removeRepository: () => {},
    updateRepository: () => {},
    agentProfileId: "",
    setAgentProfileId: () => {},
    executorId: "",
    setExecutorId: () => {},
    executorProfileId: "",
    setExecutorProfileId: () => {},
    discoveredRepositories: [],
    setDiscoveredRepositories: () => {},
    discoverReposLoading: false,
    setDiscoverReposLoading: () => {},
    discoverReposLoaded: false,
    setDiscoverReposLoaded: () => {},
    selectedWorkflowId: null,
    setSelectedWorkflowId: () => {},
    fetchedSteps: null,
    setFetchedSteps: () => {},
    isCreatingSession: false,
    setIsCreatingSession: () => {},
    isCreatingTask: false,
    setIsCreatingTask: () => {},
    useRemote: false,
    setUseRemote: () => {},
    remoteRepos: [],
    setRemoteRepos: () => {},
    addRemoteRepo: () => {},
    removeRemoteRepo: () => {},
    updateRemoteRepo: () => {},
    branchesByUrl: {
      branches: () => [],
      loading: () => false,
      error: () => undefined,
      ensure: () => undefined,
      clear: () => undefined,
    },
    prInfoByUrl: {
      info: () => undefined,
      loading: () => false,
      settled: () => true,
      error: () => undefined,
      ensure: () => undefined,
      clear: () => undefined,
    },
    githubUrlError: null,
    setGitHubUrlError: () => {},
    workflowAgentProfileId: "",
    setWorkflowAgentProfileId: () => {},
    clearDraft: () => {},
    freshBranchEnabled: false,
    setFreshBranchEnabled: () => {},
    currentLocalBranch: "",
    setCurrentLocalBranch: () => {},
    currentLocalBranchLoading: false,
    setCurrentLocalBranchLoading: () => {},
    noRepository: false,
    setNoRepository: () => {},
    workspacePath: "",
    setWorkspacePath: () => {},
  };
}

describe("DialogPromptSection (CLI-mode parity)", () => {
  it("keeps a started task prompt locked", () => {
    taskFormInputsCalls.length = 0;
    const view = render(
      <DialogPromptSection
        isSessionMode={false}
        isTaskStarted={true}
        initialDescription="Original prompt"
        fs={makeFs()}
        handleKeyDown={(() => {}) as never}
      />,
    );

    expect((view.getByTestId("task-description-textarea") as HTMLTextAreaElement).disabled).toBe(
      true,
    );
    view.unmount();
  });

  it("keeps the prompt textarea enabled when the selected profile is passthrough", () => {
    taskFormInputsCalls.length = 0;
    render(
      <DialogPromptSection
        isSessionMode={false}
        isTaskStarted={false}
        initialDescription=""
        fs={makeFs()}
        handleKeyDown={(() => {}) as never}
        descriptionPlaceholder="Write a prompt for the agent..."
      />,
    );

    const textarea = screen.getByTestId("task-description-textarea") as HTMLTextAreaElement;
    expect(textarea.disabled).toBe(false);
    expect(textarea.placeholder).toBe("Write a prompt for the agent...");
    const last = taskFormInputsCalls.at(-1)!;
    expect(last.disabled).toBe(false);
    expect(last.placeholder).toBe("Write a prompt for the agent...");
  });

  it("does not render the legacy 'Prompt ignored — passthrough mode active' warning", () => {
    const { container } = render(
      <DialogPromptSection
        isSessionMode={false}
        isTaskStarted={false}
        initialDescription="hello"
        fs={makeFs()}
        handleKeyDown={(() => {}) as never}
      />,
    );

    expect(container.textContent).not.toMatch(/prompt ignored/i);
    expect(container.textContent).not.toMatch(/passthrough mode/i);
  });

  it("allows Jira/Linear import in passthrough (CLI) mode", () => {
    taskFormInputsCalls.length = 0;
    render(
      <DialogPromptSection
        isSessionMode={false}
        isTaskStarted={false}
        initialDescription=""
        fs={makeFs()}
        handleKeyDown={(() => {}) as never}
        workspaceId="ws-1"
        onJiraImport={() => {}}
        onLinearImport={() => {}}
      />,
    );

    const last = taskFormInputsCalls.at(-1)!;
    expect((last.jiraImport as { disabled: boolean } | undefined)?.disabled).toBe(false);
    expect((last.linearImport as { disabled: boolean } | undefined)?.disabled).toBe(false);
  });

  it("forwards onComposerSubmit to TaskFormInputs", () => {
    taskFormInputsCalls.length = 0;
    const onComposerSubmit = () => true;
    render(
      <DialogPromptSection
        isSessionMode={false}
        isTaskStarted={false}
        initialDescription=""
        fs={makeFs()}
        handleKeyDown={(() => {}) as never}
        onComposerSubmit={onComposerSubmit}
      />,
    );

    const last = taskFormInputsCalls.at(-1)!;
    expect(last.onComposerSubmit).toBe(onComposerSubmit);
  });
});

describe("CreateEditSelectors", () => {
  it("links credential setup to the selected executor profile", () => {
    const EmptySelector = () => <button type="button">selector</button>;

    render(
      <CreateEditSelectors
        isTaskStarted={false}
        agentProfiles={[{ id: "agent-1", label: "Codex", agent_name: "codex" } as never]}
        agentProfilesLoading={false}
        agentProfileOptions={[]}
        agentProfileId=""
        onAgentProfileChange={() => {}}
        isCreatingSession={false}
        executorProfileOptions={[]}
        executorProfileId="exec-profile-1"
        onExecutorProfileChange={() => {}}
        executorsLoading={false}
        AgentSelectorComponent={EmptySelector}
        ExecutorProfileSelectorComponent={EmptySelector}
        workflowAgentLocked={false}
        noCompatibleAgent={true}
        executorProfileName="Docker"
      />,
    );

    expect(screen.getByRole("link", { name: /configure credentials/i }).getAttribute("href")).toBe(
      "/settings/executors/exec-profile-1",
    );
  });
});
