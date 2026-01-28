import { fetchJson, type ApiRequestOptions } from '../client';
import type {
  ListWorkflowTemplatesResponse,
  ListWorkflowStepsResponse,
  ListSessionStepHistoryResponse,
  StepBehaviors,
  StepDefinition,
  WorkflowTemplate,
  WorkflowStep,
  WorkflowStepType,
} from '@/lib/types/http';

type BackendWorkflowTemplateStep = {
  name: string;
  step_type: WorkflowStepType;
  position: number;
  color?: string;
  auto_start_agent?: boolean;
  plan_mode?: boolean;
  require_approval?: boolean;
  prompt_prefix?: string;
  prompt_suffix?: string;
};

type BackendWorkflowTemplate = Omit<WorkflowTemplate, 'default_steps'> & {
  steps?: BackendWorkflowTemplateStep[];
  default_steps?: BackendWorkflowTemplateStep[];
};

const toStepBehaviors = (step: BackendWorkflowTemplateStep): StepBehaviors | undefined => {
  const behaviors: StepBehaviors = {};
  if (step.auto_start_agent !== undefined) {
    behaviors.autoStartAgent = step.auto_start_agent;
  }
  if (step.plan_mode !== undefined) {
    behaviors.planMode = step.plan_mode;
  }
  if (step.require_approval !== undefined) {
    behaviors.requireApproval = step.require_approval;
  }
  if (step.prompt_prefix) {
    behaviors.promptPrefix = step.prompt_prefix;
  }
  if (step.prompt_suffix) {
    behaviors.promptSuffix = step.prompt_suffix;
  }
  return Object.keys(behaviors).length ? behaviors : undefined;
};

const normalizeWorkflowTemplate = (template: BackendWorkflowTemplate): WorkflowTemplate => {
  const steps = template.default_steps ?? template.steps ?? [];
  const default_steps: StepDefinition[] = steps.map((step) => ({
    name: step.name,
    step_type: step.step_type,
    position: step.position,
    color: step.color,
    behaviors: toStepBehaviors(step),
  }));
  return {
    ...template,
    default_steps,
  };
};

// Workflow Template operations
export async function listWorkflowTemplates(options?: ApiRequestOptions) {
  const response = await fetchJson<ListWorkflowTemplatesResponse>(
    '/api/v1/workflow/templates',
    options
  );
  return {
    ...response,
    templates: (response.templates ?? []).map((template) =>
      normalizeWorkflowTemplate(template as BackendWorkflowTemplate)
    ),
  };
}

export async function getWorkflowTemplate(templateId: string, options?: ApiRequestOptions) {
  const response = await fetchJson<WorkflowTemplate>(
    `/api/v1/workflow/templates/${templateId}`,
    options
  );
  return normalizeWorkflowTemplate(response as BackendWorkflowTemplate);
}

// Workflow Step operations
export async function listWorkflowSteps(boardId: string, options?: ApiRequestOptions) {
  return fetchJson<ListWorkflowStepsResponse>(`/api/v1/workflow/steps?board_id=${boardId}`, options);
}

export async function getWorkflowStep(stepId: string, options?: ApiRequestOptions) {
  return fetchJson<WorkflowStep>(`/api/v1/workflow/steps/${stepId}`, options);
}

export async function createWorkflowStep(
  payload: {
    board_id: string;
    name: string;
    step_type: string;
    position: number;
    color?: string;
    behaviors?: Record<string, unknown>;
  },
  options?: ApiRequestOptions
) {
  return fetchJson<WorkflowStep>('/api/v1/workflow/steps', {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

// Session Step History operations
export async function listSessionStepHistory(sessionId: string, options?: ApiRequestOptions) {
  return fetchJson<ListSessionStepHistoryResponse>(
    `/api/v1/workflow/history?session_id=${sessionId}`,
    options
  );
}
