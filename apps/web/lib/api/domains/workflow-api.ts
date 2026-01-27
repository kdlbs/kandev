import { fetchJson, type ApiRequestOptions } from '../client';
import type {
  ListWorkflowTemplatesResponse,
  ListWorkflowStepsResponse,
  ListSessionStepHistoryResponse,
  WorkflowTemplate,
  WorkflowStep,
} from '@/lib/types/http';

// Workflow Template operations
export async function listWorkflowTemplates(options?: ApiRequestOptions) {
  return fetchJson<ListWorkflowTemplatesResponse>('/api/v1/workflow/templates', options);
}

export async function getWorkflowTemplate(templateId: string, options?: ApiRequestOptions) {
  return fetchJson<WorkflowTemplate>(`/api/v1/workflow/templates/${templateId}`, options);
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

