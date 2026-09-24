import { fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";

const repoChipsProps = vi.hoisted(() => ({ current: null as Record<string, unknown> | null }));

vi.mock("@/components/enhance-prompt-button", () => ({
  EnhancePromptButton: ({ onClick }: { onClick: () => void }) => (
    <button type="button" onClick={onClick}>
      Enhance
    </button>
  ),
}));

vi.mock("@/components/task-create-dialog-repo-chips", () => ({
  RepoChipsRow: (props: Record<string, unknown>) => {
    repoChipsProps.current = props;
    return (
      <div data-testid="repo-chips-row">
        <button type="button" data-testid="repo-chip-trigger">
          Parent repository
        </button>
      </div>
    );
  },
}));

// The subtask form reads the workspace's repository sets, which needs the store.
// This suite renders the body without a StateProvider, so both are stubbed.
vi.mock("@/hooks/domains/workspace/use-repository-sets", () => ({
  useRepositorySets: () => ({ sets: [], isLoading: false, refresh: vi.fn() }),
}));

vi.mock("@/components/task-create-dialog-repository-sets-apply", () => ({
  useApplyRepositorySet: () => vi.fn(),
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  AgentSelector: () => <div data-testid="agent-profile-selector" />,
  ExecutorProfileSelector: () => <div data-testid="executor-profile-selector" />,
}));

vi.mock("@/components/task-autopilot-toggle", () => ({
  TaskAutopilotToggle: () => <div data-testid="autopilot-toggle-row" />,
}));

vi.mock("./session-dialog-shared", () => ({
  AttachButton: ({ onClick }: { onClick: () => void }) => (
    <button type="button" onClick={onClick}>
      Attach
    </button>
  ),
  ContextSelect: () => <div data-testid="context-select" />,
}));

import { PromptZone, SubtaskFormBody } from "./new-subtask-form-parts";

const GENERATED_RESULT = {
  content: "improved prompt",
  callId: "call-123",
  durationMs: 1_200,
} satisfies UtilityGenerationResult;

type FormProps = ComponentProps<typeof SubtaskFormBody>;

function createFormProps(workspaceMode: FormProps["workspaceMode"]): FormProps {
  return {
    fs: {
      repositories: [{ key: "row-1", repositoryId: "parent-repository", branch: "main" }],
      useRemote: false,
      executorProfileId: "executor-profile",
      autopilot: false,
      setAutopilot: vi.fn(),
    } as unknown as FormProps["fs"],
    handlers: {
      handleRowRepositoryChange: vi.fn(),
      handleRowBranchChange: vi.fn(),
      handleToggleRemote: vi.fn(),
      handleAgentProfileChange: vi.fn(),
      handleExecutorProfileChange: vi.fn(),
    } as unknown as FormProps["handlers"],
    title: "Subtask",
    setTitle: vi.fn(),
    autopilot: false,
    workspaceId: "workspace-1",
    availableRepositories: [],
    worktreeBranch: "feature/parent",
    isLocalExecutor: false,
    freshBranchAvailable: false,
    hasAllBranches: true,
    profileOptions: [],
    executorProfileOptions: [],
    agentProfileId: "agent-profile",
    workspaceMode,
    onWorkspaceModeChange: vi.fn(),
    contextValue: "blank",
    onContextChange: vi.fn(),
    hasInitialPrompt: false,
    sessionOptions: [],
    promptZone: <div data-testid="prompt-zone" />,
    isCreating: false,
    isSummarizing: false,
    hasPrompt: true,
    onClose: vi.fn(),
    onSubmit: vi.fn(),
  };
}

describe("PromptZone", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows inline recovery controls for retained enhanced prompts", async () => {
    const onApplyPending = vi.fn();
    const onCopyPending = vi.fn();

    render(
      <PromptZone
        promptRef={{ current: null }}
        promptValue="original prompt"
        contextItems={[]}
        attachments={{
          attachments: [],
          isDragging: false,
          fileInputRef: { current: null },
          handleRemoveAttachment: vi.fn(),
          handleRetryAttachment: vi.fn(),
          handlePaste: vi.fn(),
          handleDragOver: vi.fn(),
          handleDragLeave: vi.fn(),
          handleDrop: vi.fn(),
          handleAttachClick: vi.fn(),
          handleFileInputChange: vi.fn(),
        }}
        isCreating={false}
        isSummarizing={false}
        isEnhancingPrompt={false}
        isUtilityConfigured={true}
        handleEnhancePrompt={vi.fn()}
        pendingResult={GENERATED_RESULT}
        onPromptChange={vi.fn()}
        onApplyPending={onApplyPending}
        onCopyPending={onCopyPending}
        onSubmitShortcut={vi.fn()}
      />,
    );

    expect(screen.getByTestId("prompt-result-recovery")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    expect(onApplyPending).toHaveBeenCalledTimes(1);
    expect(onCopyPending).toHaveBeenCalledTimes(1);
  });
});

describe("SubtaskFormBody workspace messaging", () => {
  beforeEach(() => {
    repoChipsProps.current = null;
  });

  it("shows the parent branch only when the child inherits the workspace", () => {
    const { rerender } = render(<SubtaskFormBody {...createFormProps("inherit_parent")} />);

    expect(screen.getByText("Same branch as current session")).toBeTruthy();

    rerender(<SubtaskFormBody {...createFormProps("new_workspace")} />);

    expect(screen.queryByText("Same branch as current session")).toBeNull();
    expect(screen.getByTestId("repo-chip-trigger")).toBeTruthy();
  });

  it("disables creation while a selected repository is unresolved", () => {
    render(<SubtaskFormBody {...createFormProps("new_workspace")} hasAllBranches={false} />);

    const submitButtons = screen.getAllByRole("button", { name: /create subtask/i });
    expect((submitButtons.at(-1) as HTMLButtonElement).disabled).toBe(true);
  });

  it("passes executor source policy and transition handlers to editable workspace subtasks", () => {
    const handlers = createFormProps("new_workspace").handlers;
    const onFolderSelectionAdded = vi.fn();
    const onRepositorySelectionAdded = vi.fn();
    const onRepositorySelectionRemoved = vi.fn();
    const onRefreshRemoteOrigins = vi.fn();
    const executorSourcePolicy = {
      capabilities: {
        canAddFolders: false,
        canChooseCheckoutBranch: true,
        requiresCloneableLocalRepository: true,
        executorCapabilitiesKnown: true,
      },
      mode: "repository" as const,
      folderAvailable: false,
      folderDisabledReason: "requires_host_executor" as const,
      incompatible: false,
      incompatibleReason: undefined,
    };
    const props = createFormProps("new_workspace");
    props.handlers = {
      ...handlers,
      onFolderSelectionAdded,
      onRepositorySelectionAdded,
      onRepositorySelectionRemoved,
    } as FormProps["handlers"];
    props.executorSourcePolicy = executorSourcePolicy;
    props.folderDisabledReason = "Folders require a host executor";
    props.remoteOriginStates = {
      "row-1": {
        status: "ready",
        result: {
          ready: true,
          origin: "https://github.com/acme/parent.git",
          branches: [{ name: "main", type: "remote", remote: "origin" }],
        },
      },
    };
    props.onRefreshRemoteOrigins = onRefreshRemoteOrigins;

    render(<SubtaskFormBody {...props} />);

    expect(repoChipsProps.current).toMatchObject({
      executorSourcePolicy,
      folderDisabledReason: "Folders require a host executor",
      remoteOriginStates: props.remoteOriginStates,
      onRefreshRemoteOrigins,
      onFolderSelectionAdded,
      onRepositorySelectionAdded,
      onRepositorySelectionRemoved,
    });
  });
});
