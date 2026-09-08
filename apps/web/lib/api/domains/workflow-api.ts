import { fetchJson, type ApiRequestOptions } from "../client";
import {
  agentProfileId,
  normalizeWorkflowProfileSessionStartPolicy,
  normalizeWorkflowProfileSessionEndPolicy,
} from "@/lib/types/http";
import type {
  ListWorkflowTemplatesResponse,
  ListWorkflowStepsResponse,
  ListSessionStepHistoryResponse,
  StepDefinition,
  StepEvents,
  WorkflowTemplate,
  WorkflowStep,
} from "@/lib/types/http";

type BackendTemplateStep = {
  id?: string;
  name: string;
  position: number;
  color?: string;
  prompt?: string;
  events?: StepEvents;
  is_start_step?: boolean;
  show_in_command_panel?: boolean;
  agent_profile_id?: string;
  profile_session_start_policy?: StepDefinition["profile_session_start_policy"];
  profile_session_end_policy?: StepDefinition["profile_session_end_policy"];
  auto_advance_requires_signal?: boolean;
  cancel_triggers_turn_complete?: boolean;
  wip_limit?: number;
  pull_from_step_id?: string | null;
};

type BackendWorkflowTemplate = Omit<WorkflowTemplate, "default_steps"> & {
  steps?: BackendTemplateStep[];
  default_steps?: BackendTemplateStep[];
};

export const normalizeWorkflowTemplate = (template: BackendWorkflowTemplate): WorkflowTemplate => {
  const steps = template.default_steps ?? template.steps ?? [];
  const default_steps: StepDefinition[] = steps.map((step) => ({
    id: step.id,
    name: step.name,
    position: step.position,
    color: step.color,
    prompt: step.prompt,
    events: step.events,
    is_start_step: step.is_start_step,
    show_in_command_panel: step.show_in_command_panel,
    agent_profile_id: step.agent_profile_id ? agentProfileId(step.agent_profile_id) : undefined,
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      step.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      step.profile_session_end_policy,
    ),
    auto_advance_requires_signal: step.auto_advance_requires_signal,
    cancel_triggers_turn_complete: step.cancel_triggers_turn_complete,
    wip_limit: step.wip_limit,
    pull_from_step_id: step.pull_from_step_id ?? null,
  }));
  return {
    ...template,
    default_steps,
  };
};

// Workflow Template operations
export async function listWorkflowTemplates(options?: ApiRequestOptions) {
  const response = await fetchJson<ListWorkflowTemplatesResponse>(
    "/api/v1/workflow/templates",
    options,
  );
  return {
    ...response,
    templates: (response.templates ?? []).map((template) =>
      normalizeWorkflowTemplate(template as BackendWorkflowTemplate),
    ),
  };
}

export async function getWorkflowTemplate(templateId: string, options?: ApiRequestOptions) {
  const response = await fetchJson<WorkflowTemplate>(
    `/api/v1/workflow/templates/${templateId}`,
    options,
  );
  return normalizeWorkflowTemplate(response as BackendWorkflowTemplate);
}

// Workflow Step operations
export async function listWorkflowSteps(workflowId: string, options?: ApiRequestOptions) {
  const response = await fetchJson<ListWorkflowStepsResponse>(
    `/api/v1/workflows/${workflowId}/workflow/steps`,
    options,
  );
  return {
    ...response,
    steps: response.steps.map((step) => ({
      ...step,
      profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
        step.profile_session_start_policy,
      ),
      profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
        step.profile_session_end_policy,
      ),
    })),
  };
}

export async function getWorkflowStep(stepId: string, options?: ApiRequestOptions) {
  const step = await fetchJson<WorkflowStep>(`/api/v1/workflow/steps/${stepId}`, options);
  return {
    ...step,
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      step.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      step.profile_session_end_policy,
    ),
  };
}

export async function createWorkflowStep(
  payload: {
    workflow_id: string;
    name: string;
    position: number;
    color?: string;
    prompt?: string;
    events?: StepEvents;
    cancel_triggers_turn_complete?: boolean;
    agent_profile_id?: string;
    profile_session_start_policy?: StepDefinition["profile_session_start_policy"];
    profile_session_end_policy?: StepDefinition["profile_session_end_policy"];
    wip_limit?: number;
    pull_from_step_id?: string | null;
  },
  options?: ApiRequestOptions,
) {
  const step = await fetchJson<WorkflowStep>("/api/v1/workflow/steps", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
  return {
    ...step,
    profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
      step.profile_session_start_policy,
    ),
    profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
      step.profile_session_end_policy,
    ),
  };
}

// Session Step History operations
export async function listSessionStepHistory(sessionId: string, options?: ApiRequestOptions) {
  return fetchJson<ListSessionStepHistoryResponse>(
    `/api/v1/sessions/${encodeURIComponent(sessionId)}/workflow/history`,
    options,
  );
}
