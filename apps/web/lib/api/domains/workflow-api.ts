import { fetchJson, type ApiRequestOptions } from '../client';
import type {
  ListWorkflowTemplatesResponse,
  ListWorkflowStepsResponse,
  ListSessionStepHistoryResponse,
  StepDefinition,
  StepEvents,
  WorkflowTemplate,
  WorkflowStep,
} from '@/lib/types/http';

type BackendTemplateStep = {
  name: string;
  position: number;
  color?: string;
  prompt?: string;
  events?: StepEvents;
};

type BackendWorkflowTemplate = Omit<WorkflowTemplate, 'default_steps'> & {
  steps?: BackendTemplateStep[];
  default_steps?: BackendTemplateStep[];
};

const normalizeWorkflowTemplate = (template: BackendWorkflowTemplate): WorkflowTemplate => {
  const steps = template.default_steps ?? template.steps ?? [];
  const default_steps: StepDefinition[] = steps.map((step) => ({
    name: step.name,
    position: step.position,
    color: step.color,
    prompt: step.prompt,
    events: step.events,
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
export async function listWorkflowSteps(workflowId: string, options?: ApiRequestOptions) {
  return fetchJson<ListWorkflowStepsResponse>(`/api/v1/workflows/${workflowId}/workflow/steps`, options);
}

export async function getWorkflowStep(stepId: string, options?: ApiRequestOptions) {
  return fetchJson<WorkflowStep>(`/api/v1/workflow/steps/${stepId}`, options);
}

export async function createWorkflowStep(
  payload: {
    workflow_id: string;
    name: string;
    position: number;
    color?: string;
    prompt?: string;
    events?: StepEvents;
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
