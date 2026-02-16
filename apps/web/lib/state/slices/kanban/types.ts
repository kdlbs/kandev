import type { TaskState as TaskStatus } from '@/lib/types/http';

export type KanbanStepEvents = {
  on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_turn_start?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
};

export type KanbanState = {
  workflowId: string | null;
  steps: Array<{
    id: string;
    title: string;
    color: string;
    position: number;
    events?: KanbanStepEvents;
    allow_manual_move?: boolean;
    prompt?: string;
    is_start_step?: boolean;
  }>;
  tasks: Array<{
    id: string;
    workflowStepId: string;
    title: string;
    description?: string;
    position: number;
    state?: TaskStatus;
    repositoryId?: string;
    primarySessionId?: string | null;
    sessionCount?: number | null;
    reviewStatus?: 'pending' | 'approved' | 'changes_requested' | 'rejected' | null;
    updatedAt?: string;
  }>;
  isLoading?: boolean;
};

export type WorkflowSnapshotData = {
  workflowId: string;
  workflowName: string;
  steps: KanbanState['steps'];
  tasks: KanbanState['tasks'];
};

export type KanbanMultiState = {
  snapshots: Record<string, WorkflowSnapshotData>;
  isLoading: boolean;
};

export type WorkflowsState = {
  items: Array<{ id: string; workspaceId: string; name: string; description?: string | null }>;
  activeId: string | null;
};

export type TaskState = {
  activeTaskId: string | null;
  activeSessionId: string | null;
};

export type KanbanSliceState = {
  kanban: KanbanState;
  kanbanMulti: KanbanMultiState;
  workflows: WorkflowsState;
  tasks: TaskState;
};

export type KanbanSliceActions = {
  setActiveWorkflow: (workflowId: string | null) => void;
  setWorkflows: (workflows: WorkflowsState['items']) => void;
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
  clearActiveSession: () => void;
  setWorkflowSnapshot: (workflowId: string, data: WorkflowSnapshotData) => void;
  setKanbanMultiLoading: (loading: boolean) => void;
  clearKanbanMulti: () => void;
  updateMultiTask: (workflowId: string, task: KanbanState['tasks'][number]) => void;
  removeMultiTask: (workflowId: string, taskId: string) => void;
};

export type KanbanSlice = KanbanSliceState & KanbanSliceActions;
