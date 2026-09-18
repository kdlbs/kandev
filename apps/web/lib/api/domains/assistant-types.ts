import type { ClarificationAnswer, ClarificationQuestion } from "@/lib/types/http";
import type { Improvement } from "./assistant-maintenance-types";
export type AssistantMode = "answer" | "inspect" | "design" | "execute";
export type AssistantBinding = {
  id: string;
  owner_user_id: string;
  orchestrator_id: string;
  home_workspace_id: string;
  conversation_id: string;
  version: number;
  execution_mode: AssistantMode;
  intent_revision: number;
  paused: boolean;
  authority_reason?: string;
  authority?: {
    profile_id: string;
    executor_id: string;
    mode: AssistantMode;
    unsupported_reason?: string;
  };
};
export type AssistantOperation = {
  operation_id: string;
  expected_intent_revision: number;
  expected_binding_version: number;
};
export type AssistantAttention = {
  id: string;
  binding_id: string;
  workspace_id: string;
  task_id: string;
  session_id: string;
  source_id: string;
  source_revision: string;
  revision: number;
  summary: string;
  updated_at: string;
  kind: "question" | "permission" | "authentication" | "failure" | "review" | "result";
  state: "pending" | "resolved" | "expired" | "unknown" | "inactive";
};
export type AssistantInput = {
  source_id: string;
  kind: AssistantAttention["kind"];
  state: AssistantAttention["state"];
  source_revision: string;
  task_id: string;
  session_id: string;
  pending_id: string;
  request_id?: string;
  profile_id: string;
  summary: string;
  questions?: Array<ClarificationQuestion & { assistant_delegable?: boolean }>;
  options?: Array<{ option_id: string; label: string; kind: string }>;
  permission?: {
    title: string;
    task_id: string;
    session_id: string;
    pending_id: string;
    request_id: string;
    action: {
      type: string;
      description?: string;
      command?: string;
      cwd?: string;
      path?: string;
      destination?: string;
      server?: string;
      tool?: string;
      redacted: boolean;
    };
    options: Array<{ option_id: string; name: string; kind: string }>;
  };
};
export type AssistantInputSnapshot = { attention: AssistantAttention; input: AssistantInput };
export type AssistantAnswer = AssistantOperation & {
  expected_revision: number;
  source_revision: string;
  session_id: string;
  answers?: ClarificationAnswer[];
  option_id?: string;
  rejected?: boolean;
  reject_reason?: string;
};
export type AssistantObjective = {
  id: string;
  title: string;
  mode: AssistantMode;
  revision: number;
  acceptance_revision: number;
  source_comment_id: string;
  status:
    | "active"
    | "waiting_user"
    | "waiting_dependency"
    | "ready_for_review"
    | "complete"
    | "paused"
    | "cancelled";
  acceptance: Array<{ id: string; description: string }>;
  evidence: Array<{
    criterion_id: string;
    source_kind: "comment" | "task_message";
    source_id: string;
    task_id?: string;
    session_id?: string;
    acceptance_revision: number;
  }>;
};
export type AssistantMemory = {
  id: string;
  key: string;
  content: string;
  owner_user_id: string;
  source_comment_id: string;
  scope: "user" | "workspace" | "project" | "task" | "environment";
  scope_id: string;
  revision: number;
  confirmed: boolean;
  priority: number;
  expires_at?: string;
  updated_at: string;
};
export type AssistantMemoryEdit = Pick<
  AssistantMemory,
  "key" | "content" | "scope" | "scope_id" | "source_comment_id" | "confirmed"
> & { expected_revision: number; priority?: number; expires_at?: string | null };
export type AssistantCapability = {
  id: string;
  kind: string;
  name: string;
  effect: string;
  health: string;
  reason: string;
  configured: boolean;
  attached: boolean;
  inspect_allowed: boolean;
  profile_id?: string;
  session_id?: string;
};
export type AssistantCredential = {
  id: string;
  purpose: string;
  account: string;
  environment: string;
  profile_id: string;
  scope: string;
  health: string;
  unblock_action?: string;
  expires_at?: string;
  validation: { status: string; reason: string; validated_at?: string };
};
export type AssistantPage<T> = { entries: T[]; next_cursor: string };
export type AssistantPages = {
  improvements: Improvement;
  attention: AssistantAttention;
  objectives: AssistantObjective;
  memory: AssistantMemory;
  capabilities: AssistantCapability;
};
export type AssistantControl = AssistantOperation & {
  action: "pause" | "resume" | "stop_managed_work";
  after?: string;
};
export type AssistantControlReceipt = {
  paused?: boolean;
  binding_version?: number;
  intent_revision?: number;
  partial?: boolean;
  next_cursor?: string;
  sessions?: Array<{
    task_id: string;
    session_id: string;
    status: "stopped" | "already_finished" | "failed" | "unknown";
  }>;
};
