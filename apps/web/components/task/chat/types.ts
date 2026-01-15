'use client';

export type ToolCallMetadata = {
  tool_call_id?: string;
  tool_name?: string;
  title?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  args?: Record<string, unknown>;
  result?: string;
};

export type StatusMetadata = {
  progress?: number;
  status?: string;
  stage?: string;
  message?: string;
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
  hunks: unknown[];
  oldFile?: { fileName?: string; fileLang?: string };
  newFile?: { fileName?: string; fileLang?: string };
};
