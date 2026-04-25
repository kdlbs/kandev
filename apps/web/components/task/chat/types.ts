"use client";

export type SubagentTaskPayload = {
  description?: string;
  prompt?: string;
  subagent_type?: string;
};

export type GenericPayload = {
  name?: string;
  input?: unknown;
  output?: unknown;
};

// MonitorView is the structured shape the kandev ACP adapter writes into the
// Generic tool payload's `output.monitor` field for Claude-acp's `Monitor`
// tool. The adapter mutates this in place across tool_call_updates so the
// UI sees a stable view of the Monitor's state (event count, recent tail,
// terminal flag) without needing a custom NormalizedPayload kind.
export type MonitorView = {
  kind?: string;
  task_id?: string;
  command?: string;
  event_count?: number;
  recent_events?: string[];
  ended?: boolean;
  end_reason?: string;
};

// Output wrapper helper: when the adapter attaches a Monitor view, it lands
// at `generic.output.monitor`. Frontend code should narrow via this guard
// rather than casting on its own.
export function readMonitorView(payload: GenericPayload | undefined): MonitorView | null {
  const out = payload?.output;
  if (!out || typeof out !== "object") return null;
  const wrapper = out as { monitor?: unknown };
  if (!wrapper.monitor || typeof wrapper.monitor !== "object") return null;
  return wrapper.monitor as MonitorView;
}

export type ReadFileOutput = {
  content?: string;
  line_count?: number;
  truncated?: boolean;
  language?: string;
};

export type ReadFilePayload = {
  file_path?: string;
  offset?: number;
  limit?: number;
  output?: ReadFileOutput;
};

export type CodeSearchOutput = {
  files?: string[];
  file_count?: number;
  truncated?: boolean;
};

export type CodeSearchPayload = {
  query?: string;
  pattern?: string;
  path?: string;
  glob?: string;
  output?: CodeSearchOutput;
};

export type FileMutation = {
  type?: string;
  content?: string;
  old_content?: string;
  new_content?: string;
  diff?: string;
};

export type ModifyFilePayload = {
  file_path?: string;
  mutations?: FileMutation[];
};

export type ShellExecOutput = {
  exit_code?: number;
  stdout?: string;
  stderr?: string;
};

export type ShellExecPayload = {
  command?: string;
  work_dir?: string;
  description?: string;
  timeout?: number;
  background?: boolean;
  output?: ShellExecOutput;
};

export type HttpRequestPayload = {
  url?: string;
  method?: string;
  response?: string;
  is_error?: boolean;
};

export type NormalizedPayload = {
  kind?: string;
  subagent_task?: SubagentTaskPayload;
  generic?: GenericPayload;
  read_file?: ReadFilePayload;
  code_search?: CodeSearchPayload;
  modify_file?: ModifyFilePayload;
  shell_exec?: ShellExecPayload;
  http_request?: HttpRequestPayload;
};

export type ToolCallMetadata = {
  tool_call_id?: string;
  parent_tool_call_id?: string; // For subagent nesting
  tool_name?: string;
  title?: string;
  status?: "pending" | "running" | "complete" | "error";
  args?: Record<string, unknown>;
  result?: string;
  normalized?: NormalizedPayload;
};

export type StatusMetadata = {
  progress?: number;
  status?: string;
  stage?: string;
  message?: string;
  variant?: "default" | "warning" | "error";
  cancelled?: boolean;
};

export type RecoveryAuthMethod = {
  id: string;
  name: string;
  description?: string;
  terminal_auth?: { command: string; args?: string[]; label?: string };
  meta?: Record<string, unknown>;
};

export type RecoveryMetadata = StatusMetadata & {
  recovery_actions: true;
  session_id: string;
  task_id: string;
  has_resume_token: boolean;
  is_auth_error?: boolean;
  auth_methods?: RecoveryAuthMethod[];
};

export type MessageAction = {
  type: "archive_task" | "delete_task" | "ws_request";
  label: string;
  tooltip?: string;
  variant?: "default" | "destructive";
  icon?: string;
  params?: Record<string, unknown>;
  test_id?: string;
};

export type TodoMetadata = { text: string; done?: boolean } | string;

export type ContentBlock = {
  type: string; // "text", "image", "audio", "resource_link", "resource"
  text?: string;
  data?: string; // base64 for image/audio
  mime_type?: string;
  uri?: string;
  name?: string;
  title?: string;
  description?: string;
  size?: number;
};

export type RichMetadata = {
  thinking?: string;
  todos?: TodoMetadata[];
  diff?: unknown;
  content_blocks?: ContentBlock[];
};

export type DiffPayload = {
  hunks: string[];
  oldFile?: { fileName?: string; fileLang?: string };
  newFile?: { fileName?: string; fileLang?: string };
};
