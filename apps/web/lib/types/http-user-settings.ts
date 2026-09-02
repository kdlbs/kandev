import type { WorkspaceId } from "./ids";

export type MCPTaskAgentProfileDefault = "current_task" | "workspace_default";
export type StartupPage = "task_overview" | "last_task";
export type LspStatusLocation = "toolbar" | "status_bar";
export type LastSeenDisplay = "absolute" | "relative";

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
  task_row?: SidebarTaskRowPresentationApi | null;
};

export type SidebarViewDraftApi = {
  base_view_id: string;
  filters: Array<{ id: string; dimension: string; op: string; value: unknown }>;
  sort: { key: string; direction: string };
  group: string;
  task_row?: SidebarTaskRowPresentationApi | null;
};

export type SidebarTaskRowPresentationApi = {
  details_enabled?: boolean;
  detail_order?: unknown[];
  visible_details?: unknown[];
  trailing?: string;
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
  workflow_ids_by_workspace?: Record<string, string>;
};

export type AppStatusBarOrderApi = {
  left_item_ids?: string[];
  right_item_ids?: string[];
};

export type UserSettings = {
  user_id: string;
  workspace_id: WorkspaceId;
  kanban_view_mode?: string;
  startup_page?: StartupPage;
  workflow_filter_id?: string;
  repository_ids: string[];
  tasks_list_sort?: string;
  tasks_list_group?: string;
  tasks_list_show_details?: boolean;
  initial_setup_complete?: boolean;
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  chat_submit_key?: "enter" | "cmd_enter";
  show_anchored_prompt_bar?: boolean;
  show_scroll_to_last_prompt?: boolean;
  show_scroll_to_start?: boolean;
  show_transcript_auto_scroll_control?: boolean;
  show_todo_list_panel?: boolean;
  show_todo_list_panel_only_when_not_empty?: boolean;
  review_auto_mark_on_scroll?: boolean;
  confirm_task_archive?: boolean;
  prevent_auto_start_agent_on_open?: boolean;
  unread_divider?: boolean;
  agent_generated_task_titles?: boolean;
  mcp_task_agent_profile_default?: MCPTaskAgentProfileDefault;
  show_release_notification?: boolean;
  release_notes_last_seen_version?: string;
  lsp_auto_start_languages?: string[];
  lsp_auto_install_languages?: string[];
  lsp_server_configs?: Record<string, Record<string, unknown>>;
  lsp_status_location?: LspStatusLocation;
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
  azure_devops_browse_preferences?: unknown;
  default_utility_agent_id?: string;
  default_utility_model?: string;
  default_utility_agent_profile_id?: string;
  keyboard_shortcuts?: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminal_link_behavior?: string;
  terminal_font_family?: string;
  terminal_font_size?: number;
  changes_panel_layout?: "flat" | "tree";
  last_seen_display?: LastSeenDisplay;
  system_metrics_display?: { show_in_topbar?: boolean; simplified?: boolean };
  app_status_bar_enabled?: boolean;
  app_status_bar_order?: AppStatusBarOrderApi;
  quick_chat_tab_order_by_workspace?: Record<string, string[]>;
  kanban_hidden_step_ids?: Record<string, string[]>;
  workflow_ids_with_auto_hide_empty_steps?: string[];
  revision?: number;
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
  startup_page?: StartupPage;
  repository_ids?: string[];
  tasks_list_sort?: string;
  tasks_list_group?: string;
  tasks_list_show_details?: boolean;
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  chat_submit_key?: "enter" | "cmd_enter";
  show_anchored_prompt_bar?: boolean;
  show_scroll_to_last_prompt?: boolean;
  show_scroll_to_start?: boolean;
  show_transcript_auto_scroll_control?: boolean;
  show_todo_list_panel?: boolean;
  show_todo_list_panel_only_when_not_empty?: boolean;
  review_auto_mark_on_scroll?: boolean;
  confirm_task_archive?: boolean;
  prevent_auto_start_agent_on_open?: boolean;
  unread_divider?: boolean;
  agent_generated_task_titles?: boolean;
  mcp_task_agent_profile_default?: MCPTaskAgentProfileDefault;
  show_release_notification?: boolean;
  release_notes_last_seen_version?: string;
  lsp_auto_start_languages?: string[];
  lsp_auto_install_languages?: string[];
  lsp_server_configs?: Record<string, Record<string, unknown>>;
  lsp_status_location?: LspStatusLocation;
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
  azure_devops_browse_preferences?: object | null;
  default_utility_agent_id?: string;
  default_utility_model?: string;
  default_utility_agent_profile_id?: string;
  keyboard_shortcuts?: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminal_link_behavior?: "new_tab" | "browser_panel";
  terminal_font_family?: string;
  terminal_font_size?: number;
  changes_panel_layout?: "flat" | "tree";
  last_seen_display?: LastSeenDisplay;
  system_metrics_display?: { show_in_topbar?: boolean; simplified?: boolean };
  app_status_bar_enabled?: boolean;
  app_status_bar_order?: AppStatusBarOrderApi;
  quick_chat_tab_order_by_workspace?: Record<string, string[]>;
  kanban_hidden_step_ids?: Record<string, string[]>;
  workflow_ids_with_auto_hide_empty_steps?: string[];
};
