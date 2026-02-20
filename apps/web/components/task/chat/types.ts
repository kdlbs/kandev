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
