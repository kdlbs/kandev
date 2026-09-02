import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskCreateDialogProps } from "./task-create-dialog";
import type { RepositoryBranchesState } from "@/lib/state/slices/workspace/types";

const mocks = vi.hoisted(() => ({
  agentGeneratedTaskTitles: false,
  submit: vi.fn(),
  submitDeps: {} as Record<string, unknown>,
}));

vi.mock("@/lib/keyboard/constants", () => ({ SHORTCUTS: { SUBMIT: "submit" } }));
vi.mock("@/hooks/use-is-utility-configured", () => ({ useIsUtilityConfigured: () => false }));
vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcutHandler: () => vi.fn(),
}));
vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({ enhancePrompt: vi.fn(), isEnhancingPrompt: false }),
}));
vi.mock("@/hooks/use-prompt-result-delivery", () => ({
  usePromptResultDelivery: () => ({
    pendingResult: null,
    captureScope: vi.fn(),
    deliver: vi.fn(),
    applyPending: vi.fn(),
    copyPending: vi.fn(),
  }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({
    getState: () => ({
      repositoryBranchPolicies: { revisionByRepositoryId: {} },
    }),
  }),
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      userSettings: { agentGeneratedTaskTitles: mocks.agentGeneratedTaskTitles },
      upsertRepository: vi.fn(),
      setRepositoryBranchPolicies: vi.fn(),
      setRepositoryBranchPoliciesLoading: vi.fn(),
      repositorySets: {
        itemsByWorkspaceId: {},
        loadingByWorkspaceId: {},
        loadedByWorkspaceId: {},
        revisionByWorkspaceId: {},
      },
      setRepositorySets: vi.fn(),
      setRepositorySetsLoading: vi.fn(),
    }),
}));
vi.mock("@/components/task-create-dialog-submit", () => ({
  useTaskSubmitHandlers: (deps: Record<string, unknown>) => {
    mocks.submitDeps = deps;
    return { handleSubmit: mocks.submit, pendingDiscard: null };
  },
}));
vi.mock("@/components/task-create-dialog-workflow-context", () => ({
  useResolvedTaskCreateWorkflowContext: (props: TaskCreateDialogProps) => props,
}));
vi.mock("@/components/task-create-dialog-state", () => ({
  computeIsTaskStarted: () => false,
  useDialogFormState: () => ({
    taskName: "",
    setTaskName: vi.fn(),
    hasTitle: false,
    setHasTitle: vi.fn(),
    hasDescription: false,
    setHasDescription: vi.fn(),
    descriptionInputRef: { current: null },
    openCycle: 1,
    repositories: [],
    discoveredRepositories: [],
    remoteRepos: [],
    prInfoByUrl: {},
    useRemote: false,
    executorId: "",
    executorProfileId: "",
    freshBranchEnabled: false,
    noRepository: false,
    blockedBy: ["dep-1"],
    setBlockedBy: vi.fn(),
    workspacePath: "",
    isCreatingSession: false,
    isCreatingTask: false,
    setIsCreatingSession: vi.fn(),
    setIsCreatingTask: vi.fn(),
    setRepositories: vi.fn(),
    setRemoteRepos: vi.fn(),
    setAgentProfileId: vi.fn(),
    setExecutorId: vi.fn(),
    setSelectedWorkflowId: vi.fn(),
    setFetchedSteps: vi.fn(),
    clearDraft: vi.fn(),
    currentDefaults: { description: "" },
  }),
  useTaskCreateDialogData: () => ({
    workflows: [],
    agentProfiles: [],
    executors: [],
    snapshots: [],
    repositories: [],
    repositoriesLoading: false,
    refreshRepositories: vi.fn(),
    taskCreateLastUsed: {
      repositoryId: null,
      agentProfileId: null,
      executorProfileId: null,
      branch: null,
    },
    userSettingsLoaded: true,
    computed: {
      effectiveWorkflowId: null,
      effectiveDefaultStepId: null,
      effectiveAgentProfileId: "",
      compatibleAgentProfiles: [],
      authLoaded: true,
      workspaceDefaults: {},
      isLocalExecutor: true,
      isPassthroughProfile: false,
      agentProfileOptions: [],
      executorProfileOptions: [],
      agentProfilesLoading: false,
      executorsLoading: false,
      workflowAgentLocked: false,
      noCompatibleAgent: false,
      selectedExecutorProfileName: "",
      executorHint: "",
      hasRepositorySelection: false,
    },
  }),
  useTaskCreateDialogEffects: vi.fn(),
  useLockedFieldSync: vi.fn(),
  useDialogHandlers: () => ({}),
  useSessionRepoName: () => null,
}));

import { hasUnavailableSavedBase, useTaskCreateDialogSetup } from "./task-create-dialog-setup";

const props: TaskCreateDialogProps = {
  open: true,
  onOpenChange: vi.fn(),
  workspaceId: "workspace-1",
  workflowId: "workflow-1",
  defaultStepId: null,
  steps: [],
};

describe("useTaskCreateDialogSetup auto-title mode", () => {
  beforeEach(() => {
    mocks.agentGeneratedTaskTitles = false;
    mocks.submit.mockReset();
  });

  it.each([
    ["create", false, false],
    ["create", true, true],
    ["edit", true, false],
    ["session", true, false],
  ] as const)("derives autoTitle=%s for %s mode with setting=%s", (mode, enabled, expected) => {
    mocks.agentGeneratedTaskTitles = enabled;
    const { result } = renderHook(() => useTaskCreateDialogSetup({ ...props, mode }));

    expect(result.current.autoTitle).toBe(expected);
  });
});

it("forwards the selected dependencies to the submit handlers", () => {
  // The payload builder handled blocked_by correctly all along; the break was
  // this hop — useSubmitHandlersWiring never passed blockedBy through, so the
  // create dialog's selection silently never reached the request. Assert the
  // hop itself, not just the leaf.
  renderHook(() => useTaskCreateDialogSetup({ ...props, mode: "create" }));
  expect(mocks.submitDeps.blockedBy).toEqual(["dep-1"]);
});

it("validates saved bases against qualified branch option values", () => {
  const repositoryBranches = {
    itemsByRepositoryId: {
      "repo-1": [{ name: "main", type: "remote", remote: "origin" }],
    },
    loadedByRepositoryId: { "repo-1": true },
  } as unknown as RepositoryBranchesState;

  expect(
    hasUnavailableSavedBase(
      [{ key: "r0", repositoryId: "repo-1", branch: "", baseBranch: "main" }],
      repositoryBranches,
    ),
  ).toBe(true);
  expect(
    hasUnavailableSavedBase(
      [{ key: "r0", repositoryId: "repo-1", branch: "", baseBranch: "origin/main" }],
      repositoryBranches,
    ),
  ).toBe(false);
});
