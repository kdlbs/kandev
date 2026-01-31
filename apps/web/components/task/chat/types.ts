'use client';

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

export type NormalizedPayload = {
  kind?: string;
  subagent_task?: SubagentTaskPayload;
  generic?: GenericPayload;
};

export type ToolCallMetadata = {
  tool_call_id?: string;
  tool_name?: string;
  title?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  args?: Record<string, unknown>;
  result?: string;
  normalized?: NormalizedPayload;
};

export type StatusMetadata = {
  progress?: number;
  status?: string;
  stage?: string;
  message?: string;
  variant?: 'default' | 'warning' | 'error';
  cancelled?: boolean;
};

export type TodoMetadata =
  | { text: string; done?: boolean }
  | string;

export type RichMetadata = {
  thinking?: string;
  todos?: TodoMetadata[];
  diff?: unknown;
};

export type DiffPayload = {
  hunks: string[];
  oldFile?: { fileName?: string; fileLang?: string };
  newFile?: { fileName?: string; fileLang?: string };
};
