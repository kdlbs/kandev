import type { StateCreator } from "zustand";
import { DEFAULT_VOICE_MODE_STATE, type SettingsSlice, type SettingsSliceState } from "./types";

export const defaultSettingsState: SettingsSliceState = {
  userSettings: {
    workspaceId: null,
    kanbanViewMode: null,
    workflowId: null,
    repositoryIds: [],
    preferredShell: null,
    shellOptions: [],
    defaultEditorId: null,
    enablePreviewOnClick: false,
    terminalLinkBehavior: "new_tab",
    chatSubmitKey: "cmd_enter",
    reviewAutoMarkOnScroll: true,
    showReleaseNotification: true,
    releaseNotesLastSeenVersion: null,
    lspAutoStartLanguages: [],
    lspAutoInstallLanguages: [],
    lspServerConfigs: {},
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
    terminalFontFamily: null,
    terminalFontSize: null,
    changesPanelLayout: "tree",
    systemMetricsDisplay: { showInTopbar: false },
    voiceMode: { ...DEFAULT_VOICE_MODE_STATE },
    loaded: false,
  },
};

type ImmerSet = Parameters<
  StateCreator<SettingsSlice, [["zustand/immer", never]], [], SettingsSlice>
>[0];

function createCoreActions(set: ImmerSet): Pick<SettingsSlice, "setUserSettings"> {
  return {
    setUserSettings: (settings) =>
      set((draft) => {
        draft.userSettings = settings;
      }),
  };
}

export const createSettingsSlice: StateCreator<
  SettingsSlice,
  [["zustand/immer", never]],
  [],
  SettingsSlice
> = (set) => ({
  ...defaultSettingsState,
  ...createCoreActions(set),
});
