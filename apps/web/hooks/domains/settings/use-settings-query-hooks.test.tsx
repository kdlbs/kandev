/* eslint-disable max-lines-per-function */
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import { useAgentDiscovery } from "./use-agent-discovery";
import { useAvailableAgents } from "./use-available-agents";
import { useCustomPrompts } from "./use-custom-prompts";
import { useAgentCapabilities } from "./use-dynamic-models";
import { useEditors } from "./use-editors";
import { useNotificationProviders } from "./use-notification-providers";
import { useSettingsData } from "./use-settings-data";
import { useUserDisplaySettings } from "../../use-user-display-settings";

const WORKSPACE_ID = "workspace-1";

const apiMocks = vi.hoisted(() => {
  const mocks = {
    fetchDefaultScripts: vi.fn(),
    fetchDynamicModels: vi.fn(),
    fetchExecutor: vi.fn(),
    fetchSystemMetricsSettings: vi.fn(),
    fetchUserSettings: vi.fn(),
    getAgentProfileMcpConfig: vi.fn(),
    getInstallJob: vi.fn(),
    listAgentDiscovery: vi.fn(),
    listAgents: vi.fn(),
    listAllExecutorProfiles: vi.fn(),
    listAvailableAgents: vi.fn(),
    listEditors: vi.fn(),
    listExecutorProfiles: vi.fn(),
    listExecutors: vi.fn(),
    listInstallJobs: vi.fn(),
    listNotificationProviders: vi.fn(),
    listPrompts: vi.fn(),
    listScriptPlaceholders: vi.fn(),
    updateUserSettings: vi.fn(),
  };
  return mocks;
});

type ItemsState = { items: unknown[] };
type LoadingItemsState = ItemsState & { loaded: boolean; loading: boolean };
type NotificationState = {
  items: unknown[];
  events: string[];
  appriseAvailable: boolean;
  loaded: boolean;
  loading: boolean;
};
type TestState = {
  editors: LoadingItemsState;
  prompts: LoadingItemsState;
  notificationProviders: NotificationState;
  userSettings: UserSettingsState;
} & Record<string, unknown>;

const createBaseState = vi.hoisted(
  () => (): TestState => ({
    editors: { items: [], loaded: false, loading: false },
    prompts: { items: [], loaded: false, loading: false },
    notificationProviders: {
      items: [],
      events: [],
      appriseAvailable: false,
      loaded: false,
      loading: false,
    },
    settingsData: { executorsLoaded: false, agentsLoaded: false },
    userSettings: {
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
      lspAutoStartLanguages: [],
      lspAutoInstallLanguages: [],
      lspServerConfigs: {},
      voiceMode: {
        enabled: true,
        engine: "auto",
        language: "auto",
        mode: "toggle",
        autoSend: false,
        whisperWebModel: "base",
      },
      loaded: false,
    },
  }),
);

const assignActions = vi.hoisted(() => (target: TestState): void => {
  Object.assign(target, {
    setEditors: (editors: unknown[]) => {
      target.editors = { items: editors, loaded: true, loading: false };
    },
    setEditorsLoading: (loading: boolean) => {
      target.editors = { ...target.editors, loading };
    },
    setPrompts: (prompts: unknown[]) => {
      target.prompts = { items: prompts, loaded: true, loading: false };
    },
    setPromptsLoading: (loading: boolean) => {
      target.prompts = { ...target.prompts, loading };
    },
    setUserSettings: (settings: UserSettingsState) => {
      target.userSettings = settings;
    },
  });
});

const storeHarness = vi.hoisted(() => {
  let state: TestState;
  function reset() {
    state = createBaseState();
    assignActions(state);
  }
  reset();
  return {
    reset,
    getState: () => state,
  };
});

vi.mock("@/lib/api/domains/settings-api", () => apiMocks);
vi.mock("@/lib/api", () => apiMocks);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: ReturnType<typeof storeHarness.getState>) => unknown) =>
    selector(storeHarness.getState()),
  useAppStoreApi: () => ({ getState: storeHarness.getState }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: vi.fn(() => Promise.resolve({})), subscribeUser: vi.fn() }),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [], isLoading: false }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useSearchParams: () => new URLSearchParams(),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

const editor = { id: "editor-1", name: "VS Code", kind: "vscode", enabled: true };
const executor = { id: "executor-1", name: "Docker", type: "docker" };
const agent = {
  id: "agent-1",
  name: "codex",
  profiles: [{ id: "profile-1", agentDisplayName: "Codex", name: "Default" }],
};
const availableAgent = { name: "codex", display_name: "Codex", installed: true };
const tool = { name: "codex", installed: true };
const prompt = { id: "prompt-1", name: "Review", content: "Review this" };
const provider = {
  id: "provider-1",
  name: "Desktop",
  type: "desktop",
  enabled: true,
  config: {},
  events: ["task.completed"],
};
const modelConfig = {
  default_model: "gpt-5",
  supports_dynamic_models: true,
  available_models: [],
  available_modes: [],
  available_commands: [],
  current_model_id: undefined,
  current_mode_id: undefined,
};
const dynamicModelsResponse = {
  agent_name: "codex",
  status: "ok",
  models: [{ id: "gpt-5", name: "GPT-5" }],
  modes: [{ id: "code", name: "Code" }],
  commands: [{ id: "review", name: "Review", description: "Review code" }],
  current_model_id: "gpt-5",
  current_mode_id: "code",
  error: null,
};
const userSettingsResponse = {
  shell_options: [{ value: "/bin/zsh", label: "zsh" }],
  settings: {
    workspace_id: WORKSPACE_ID,
    workflow_filter_id: "workflow-1",
    repository_ids: ["repo-1"],
    preferred_shell: "/bin/zsh",
    enable_preview_on_click: true,
  },
};

beforeEach(() => {
  storeHarness.reset();
  localStorage.clear();
  vi.clearAllMocks();
  apiMocks.listAvailableAgents.mockResolvedValue({ agents: [availableAgent], tools: [tool] });
  apiMocks.listAgentDiscovery.mockResolvedValue({ agents: [{ name: "codex", installed: true }] });
  apiMocks.listEditors.mockResolvedValue({ editors: [editor] });
  apiMocks.fetchUserSettings.mockResolvedValue(userSettingsResponse);
  apiMocks.listExecutors.mockResolvedValue({ executors: [executor] });
  apiMocks.listAgents.mockResolvedValue({ agents: [agent] });
  apiMocks.listPrompts.mockResolvedValue({ prompts: [prompt] });
  apiMocks.listNotificationProviders.mockResolvedValue({
    providers: [provider],
    events: ["task.completed"],
    apprise_available: true,
  });
  apiMocks.fetchDynamicModels.mockResolvedValue(dynamicModelsResponse);
});

describe("settings query hooks", () => {
  it("loads available agents through TanStack Query", async () => {
    const queryClient = createQueryClient();
    const { result } = renderHook(() => useAvailableAgents(), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.loaded).toBe(true));

    expect(result.current.items).toEqual([availableAgent]);
    expect(result.current.tools).toEqual([tool]);
    expect(queryClient.getQueryData(qk.settings.availableAgents())).toEqual({
      agents: [availableAgent],
      tools: [tool],
    });
  });

  it("hydrates editors and user settings from query data", async () => {
    const queryClient = createQueryClient();
    const { result } = renderHook(() => useEditors(), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.loaded).toBe(true));
    await waitFor(() => expect(storeHarness.getState().userSettings.loaded).toBe(true));

    expect(result.current.editors).toEqual([editor]);
    expect(queryClient.getQueryData(qk.settings.editors())).toEqual({ editors: [editor] });
    expect(queryClient.getQueryData(qk.settings.user())).toEqual(userSettingsResponse);
    expect(storeHarness.getState().userSettings.preferredShell).toBe("/bin/zsh");
  });

  it("loads settings bootstrap data through query keys", async () => {
    const queryClient = createQueryClient();
    const { result } = renderHook(() => useSettingsData(true), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.settingsData.executorsLoaded).toBe(true));
    await waitFor(() => expect(result.current.settingsData.agentsLoaded).toBe(true));

    expect(result.current.executors).toEqual([executor]);
    expect(result.current.settingsAgents).toEqual([agent]);
    expect(result.current.agentProfiles).toEqual([
      {
        id: "profile-1",
        label: "Codex • Default",
        agent_id: "agent-1",
        agent_name: "codex",
        cli_passthrough: false,
        capability_status: undefined,
        capability_error: undefined,
      },
    ]);
    expect(result.current.settingsData).toEqual({
      agentsLoaded: true,
      capabilitiesLoaded: true,
      executorsLoaded: true,
    });
    expect(queryClient.getQueryData(qk.settings.executors())).toEqual({ executors: [executor] });
    expect(queryClient.getQueryData(qk.settings.agents())).toEqual({ agents: [agent] });
    expect(queryClient.getQueryData(qk.settings.availableAgents())).toEqual({
      agents: [availableAgent],
      tools: [tool],
    });
  });

  it("loads prompts and notification providers through query keys", async () => {
    const queryClient = createQueryClient();
    const { result: prompts } = renderHook(() => useCustomPrompts(), {
      wrapper: wrapperFor(queryClient),
    });
    const { result: notifications } = renderHook(() => useNotificationProviders(), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(prompts.current.loaded).toBe(true));
    await waitFor(() => expect(notifications.current.loaded).toBe(true));

    expect(queryClient.getQueryData(qk.settings.prompts())).toEqual({ prompts: [prompt] });
    expect(queryClient.getQueryData(qk.settings.notificationProviders())).toEqual({
      providers: [provider],
      events: ["task.completed"],
      apprise_available: true,
    });
  });

  it("loads agent discovery and user display settings through query keys", async () => {
    const queryClient = createQueryClient();
    const { result: discovery } = renderHook(() => useAgentDiscovery(), {
      wrapper: wrapperFor(queryClient),
    });
    const { result: display } = renderHook(
      () => useUserDisplaySettings({ workspaceId: null, workflowId: null }),
      { wrapper: wrapperFor(queryClient) },
    );

    await waitFor(() => expect(discovery.current.loaded).toBe(true));
    await waitFor(() => expect(display.current.settings.loaded).toBe(true));

    expect(queryClient.getQueryData(qk.settings.agentDiscovery())).toEqual({
      agents: [{ name: "codex", installed: true }],
    });
    expect(queryClient.getQueryData(qk.settings.user())).toEqual(userSettingsResponse);
    expect(display.current.settings.workspaceId).toBe(WORKSPACE_ID);
  });

  it("preserves cached task-create selections when committing the initial workspace", async () => {
    localStorage.setItem(STORAGE_KEYS.LAST_REPOSITORY_ID, JSON.stringify("repo-1"));
    localStorage.setItem(STORAGE_KEYS.LAST_BRANCH, JSON.stringify("main"));
    localStorage.setItem(STORAGE_KEYS.LAST_AGENT_PROFILE_ID, JSON.stringify("agent-profile-1"));
    localStorage.setItem(
      STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID,
      JSON.stringify("executor-profile-1"),
    );
    storeHarness.getState().userSettings = {
      ...storeHarness.getState().userSettings,
      loaded: true,
      workspaceId: null,
      workflowId: "workflow-1",
    };

    const queryClient = createQueryClient();
    renderHook(
      () => useUserDisplaySettings({ workspaceId: WORKSPACE_ID, workflowId: "workflow-1" }),
      {
        wrapper: wrapperFor(queryClient),
      },
    );

    await waitFor(() =>
      expect(storeHarness.getState().userSettings.workspaceId).toBe(WORKSPACE_ID),
    );
    expect(storeHarness.getState().userSettings.taskCreateLastUsed).toEqual({
      repositoryId: "repo-1",
      branch: "main",
      agentProfileId: "agent-profile-1",
      executorProfileId: "executor-profile-1",
    });
  });

  it("loads dynamic model capabilities through the agent model query key", async () => {
    const queryClient = createQueryClient();
    const { result } = renderHook(() => useAgentCapabilities("codex", modelConfig), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.currentModelId).toBe("gpt-5"));

    expect(result.current.models).toEqual(dynamicModelsResponse.models);
    expect(result.current.modes).toEqual(dynamicModelsResponse.modes);
    expect(result.current.commands).toEqual(dynamicModelsResponse.commands);
    expect(queryClient.getQueryData(qk.settings.dynamicModels("codex"))).toEqual(
      dynamicModelsResponse,
    );
  });
});
