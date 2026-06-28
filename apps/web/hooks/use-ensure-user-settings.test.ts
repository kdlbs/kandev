import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";

type PendingTaskCreateLastUsed = {
  repositoryId?: string;
  branch?: string;
  agentProfileId?: string;
  executorProfileId?: string;
};

const mockFetchUserSettings = vi.fn();
const mockSetUserSettings = vi.fn((settings: UserSettingsState) => {
  mockState.userSettings = settings;
});
const mockReadPendingTaskCreateLastUsedState = vi.fn<[], PendingTaskCreateLastUsed>(() => ({
  repositoryId: undefined,
  branch: undefined,
  agentProfileId: undefined,
  executorProfileId: undefined,
}));
const mockReadSyncedTaskCreateLastUsedState = vi.fn<[], PendingTaskCreateLastUsed>(() => ({
  repositoryId: undefined,
  branch: undefined,
  agentProfileId: undefined,
  executorProfileId: undefined,
}));

type MockState = {
  userSettings: UserSettingsState;
  setUserSettings: typeof mockSetUserSettings;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: (...args: unknown[]) => mockFetchUserSettings(...args),
}));

vi.mock("@/components/task-create-dialog-handlers", () => ({
  readPendingTaskCreateLastUsedState: () => mockReadPendingTaskCreateLastUsedState(),
  readSyncedTaskCreateLastUsedState: () => mockReadSyncedTaskCreateLastUsedState(),
}));

import {
  __resetEnsureUserSettingsForTests,
  useEnsureUserSettings,
} from "./use-ensure-user-settings";

function makeUnloadedSettings(): UserSettingsState {
  return {
    workspaceId: null,
    workflowId: null,
    kanbanViewMode: null,
    repositoryIds: [],
    preferredShell: null,
    shellOptions: [],
    defaultEditorId: null,
    enablePreviewOnClick: false,
    chatSubmitKey: "cmd_enter",
    reviewAutoMarkOnScroll: true,
    showReleaseNotification: true,
    releaseNotesLastSeenVersion: null,
    savedLayouts: [],
    sidebarViews: [],
    sidebarActiveViewId: null,
    sidebarDraft: null,
    sidebarTaskPrefs: { pinnedTaskIds: [], orderedTaskIds: [], subtaskOrderByParentId: {} },
    taskCreateLastUsed: {
      repositoryId: null,
      branch: null,
      agentProfileId: null,
      executorProfileId: null,
    },
    jiraSavedViews: undefined,
    jiraTaskPresets: undefined,
    githubSavedPresets: undefined,
    githubDefaultQueryPresets: undefined,
    gitlabSavedPresets: undefined,
    defaultUtilityAgentId: null,
    keyboardShortcuts: {},
    terminalLinkBehavior: "new_tab",
    terminalFontFamily: null,
    terminalFontSize: null,
    changesPanelLayout: "tree",
    systemMetricsDisplay: { showInTopbar: false },
    voiceMode: {
      enabled: true,
      engine: "auto",
      language: "auto",
      mode: "toggle",
      autoSend: false,
      whisperWebModel: "base",
    },
    lspAutoStartLanguages: [],
    lspAutoInstallLanguages: [],
    lspServerConfigs: {},
    loaded: false,
  };
}

function userSettingsResponse(taskCreateLastUsed = {}) {
  return {
    shell_options: [],
    settings: {
      task_create_last_used: taskCreateLastUsed,
    },
  };
}

beforeEach(() => {
  __resetEnsureUserSettingsForTests();
  vi.clearAllMocks();
  mockState = {
    userSettings: makeUnloadedSettings(),
    setUserSettings: mockSetUserSettings,
  };
});

describe("useEnsureUserSettings", () => {
  it("fetches and stores user settings on first enabled mount", async () => {
    mockFetchUserSettings.mockResolvedValue(
      userSettingsResponse({ repository_id: "repo-1", branch: "main" }),
    );

    renderHook(() => useEnsureUserSettings(true));

    await waitFor(() => expect(mockSetUserSettings).toHaveBeenCalled());
    expect(mockFetchUserSettings).toHaveBeenCalledWith({ cache: "no-store" });
    expect(mockSetUserSettings.mock.calls[0]![0].loaded).toBe(true);
    expect(mockSetUserSettings.mock.calls[0]![0].taskCreateLastUsed).toMatchObject({
      repositoryId: "repo-1",
      branch: "main",
    });
  });

  it("joins an in-flight settings fetch instead of starting a duplicate request", async () => {
    let resolveFetch: (value: unknown) => void = () => undefined;
    mockFetchUserSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => useEnsureUserSettings(true));
    renderHook(() => useEnsureUserSettings(true));

    await waitFor(() => expect(mockFetchUserSettings).toHaveBeenCalledTimes(1));
    resolveFetch(userSettingsResponse());
    await waitFor(() => expect(mockSetUserSettings).toHaveBeenCalled());
  });

  it("merges only defined pending task-create fields over fetched settings", async () => {
    mockReadPendingTaskCreateLastUsedState.mockReturnValue({
      repositoryId: undefined,
      branch: undefined,
      agentProfileId: "agent-2",
      executorProfileId: undefined,
    });
    mockFetchUserSettings.mockResolvedValue(
      userSettingsResponse({
        repository_id: "repo-1",
        branch: "main",
        agent_profile_id: "agent-1",
        executor_profile_id: "exec-profile-1",
      }),
    );

    renderHook(() => useEnsureUserSettings(true));

    await waitFor(() => expect(mockSetUserSettings).toHaveBeenCalled());
    expect(mockSetUserSettings.mock.calls[0]![0].taskCreateLastUsed).toEqual({
      repositoryId: "repo-1",
      branch: "main",
      agentProfileId: "agent-2",
      executorProfileId: "exec-profile-1",
    });
  });

  it("preserves pending task-create fields when multiple callers join the same fetch", async () => {
    let resolveFetch: (value: unknown) => void = () => undefined;
    mockReadPendingTaskCreateLastUsedState.mockReturnValue({
      repositoryId: undefined,
      branch: "feature",
      agentProfileId: undefined,
      executorProfileId: undefined,
    });
    mockFetchUserSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => useEnsureUserSettings(true));
    renderHook(() => useEnsureUserSettings(true));
    resolveFetch(
      userSettingsResponse({
        repository_id: "repo-1",
        branch: "main",
        agent_profile_id: "agent-1",
      }),
    );

    await waitFor(() => expect(mockSetUserSettings).toHaveBeenCalledTimes(2));
    expect(mockSetUserSettings.mock.calls.at(-1)![0].taskCreateLastUsed).toMatchObject({
      repositoryId: "repo-1",
      branch: "feature",
      agentProfileId: "agent-1",
    });
  });

  it("does not fetch while disabled", () => {
    const { result } = renderHook(() => useEnsureUserSettings(false));

    expect(mockFetchUserSettings).not.toHaveBeenCalled();
    expect(result.current.loaded).toBe(false);
  });

  it("settles a failed fetch for the current mount but retries on the next mount", async () => {
    mockFetchUserSettings
      .mockRejectedValueOnce(new Error("temporary"))
      .mockResolvedValueOnce(userSettingsResponse({ repository_id: "repo-2" }));

    const first = renderHook(() => useEnsureUserSettings(true));

    await waitFor(() => expect(first.result.current.loaded).toBe(true));
    expect(mockSetUserSettings).not.toHaveBeenCalled();
    first.unmount();

    renderHook(() => useEnsureUserSettings(true));

    await waitFor(() => expect(mockFetchUserSettings).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(mockSetUserSettings).toHaveBeenCalled());
    expect(mockSetUserSettings.mock.calls[0]![0].taskCreateLastUsed.repositoryId).toBe("repo-2");
  });
});

describe("useEnsureUserSettings — stale settings fetches", () => {
  it("keeps task-create edits made while a settings fetch is in flight", async () => {
    let resolveFetch: (value: unknown) => void = () => undefined;
    mockFetchUserSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );

    renderHook(() => useEnsureUserSettings(true));
    await waitFor(() => expect(mockFetchUserSettings).toHaveBeenCalledTimes(1));

    mockReadSyncedTaskCreateLastUsedState.mockReturnValue({
      repositoryId: "repo-2",
      branch: "feature",
      agentProfileId: undefined,
      executorProfileId: undefined,
    });
    resolveFetch(
      userSettingsResponse({
        repository_id: "repo-1",
        branch: "main",
      }),
    );

    await waitFor(() => expect(mockSetUserSettings).toHaveBeenCalled());
    expect(mockSetUserSettings.mock.calls[0]![0].taskCreateLastUsed).toMatchObject({
      repositoryId: "repo-2",
      branch: "feature",
    });
  });
});
