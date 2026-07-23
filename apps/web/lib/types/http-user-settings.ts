import type { WorkspaceId } from "./ids";
import type { VoiceModeSettings } from "./http-voice";

export type MCPTaskAgentProfileDefault = "current_task" | "workspace_default";

export type SavedLayout = {
  id: string;
  name: string;
  is_default: boolean;
  layout: Record<string, unknown>;
  created_at: string;
};

export type SidebarViewApi = {
  id: string;
  name: string;
  filters: Array<{ id: string; dimension: string; op: string; value: unknown }>;
  sort: { key: string; direction: string };
  group: string;
  collapsed_groups: string[];
};

export type SidebarViewDraftApi = {
  base_view_id: string;
  filters: Array<{ id: string; dimension: string; op: string; value: unknown }>;
  sort: { key: string; direction: string };
  group: string;
};

export type SidebarTaskPrefsApi = {
  pinned_task_ids: string[];
  ordered_task_ids: string[];
  subtask_order_by_parent_id: Record<string, string[]>;
};

export type TaskCreateLastUsedApi = {
  repository_id?: string;
  branch?: string;
  agent_profile_id?: string;
  executor_profile_id?: string;
};

export type AppStatusBarOrderApi = {
  left_item_ids?: string[];
  right_item_ids?: string[];
};

export type UserSettings = {
  user_id: string;
  workspace_id: WorkspaceId;
  kanban_view_mode?: string;
  workflow_filter_id?: string;
  repository_ids: string[];
  tasks_list_sort?: string;
  tasks_list_group?: string;
  initial_setup_complete?: boolean;
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  chat_submit_key?: "enter" | "cmd_enter";
  review_auto_mark_on_scroll?: boolean;
  confirm_task_archive?: boolean;
  mcp_task_agent_profile_default?: MCPTaskAgentProfileDefault;
  show_release_notification?: boolean;
  release_notes_last_seen_version?: string;
  lsp_auto_start_languages?: string[];
  lsp_auto_install_languages?: string[];
  lsp_server_configs?: Record<string, Record<string, unknown>>;
  saved_layouts?: SavedLayout[];
  sidebar_views?: SidebarViewApi[];
  sidebar_active_view_id?: string;
  sidebar_draft?: SidebarViewDraftApi | null;
  sidebar_task_prefs?: SidebarTaskPrefsApi;
  task_create_last_used?: TaskCreateLastUsedApi;
  jira_saved_views?: unknown;
  jira_task_presets?: unknown;
  github_saved_presets?: unknown;
  github_default_query_presets?: unknown;
  gitlab_saved_presets?: unknown;
  default_utility_agent_id?: string;
  default_utility_model?: string;
  keyboard_shortcuts?: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminal_link_behavior?: string;
  terminal_font_family?: string;
  terminal_font_size?: number;
  changes_panel_layout?: "flat" | "tree";
  system_metrics_display?: { show_in_topbar?: boolean };
  app_status_bar_order?: AppStatusBarOrderApi;
  voice_mode?: VoiceModeSettings;
  updated_at: string;
};

export type UserSettingsResponse = {
  settings: UserSettings;
  shell_options?: Array<{ value: string; label: string }>;
};

export type UserSettingsUpdatePayload = {
  workspace_id?: string;
  workflow_filter_id?: string;
  kanban_view_mode?: string;
  repository_ids?: string[];
  tasks_list_sort?: string;
  tasks_list_group?: string;
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  chat_submit_key?: "enter" | "cmd_enter";
  review_auto_mark_on_scroll?: boolean;
  confirm_task_archive?: boolean;
  mcp_task_agent_profile_default?: MCPTaskAgentProfileDefault;
  show_release_notification?: boolean;
  release_notes_last_seen_version?: string;
  lsp_auto_start_languages?: string[];
  lsp_auto_install_languages?: string[];
  lsp_server_configs?: Record<string, Record<string, unknown>>;
  saved_layouts?: SavedLayout[];
  sidebar_views?: SidebarViewApi[];
  sidebar_active_view_id?: string;
  sidebar_draft?: SidebarViewDraftApi | null;
  sidebar_task_prefs?: SidebarTaskPrefsApi;
  task_create_last_used?: TaskCreateLastUsedApi;
  jira_saved_views?: unknown[] | null;
  jira_task_presets?: unknown[] | null;
  github_saved_presets?: unknown[] | null;
  github_default_query_presets?: object | null;
  gitlab_saved_presets?: unknown[] | null;
  default_utility_agent_id?: string;
  default_utility_model?: string;
  keyboard_shortcuts?: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminal_link_behavior?: "new_tab" | "browser_panel";
  terminal_font_family?: string;
  terminal_font_size?: number;
  changes_panel_layout?: "flat" | "tree";
  system_metrics_display?: { show_in_topbar?: boolean };
  app_status_bar_order?: AppStatusBarOrderApi;
  voice_mode?: VoiceModeSettings;
};
