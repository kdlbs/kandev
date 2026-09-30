import type { KanbanState } from "./kanban/types";

export type TaskOverview = KanbanState["tasks"][number];
export type TaskOverviewPatch = Partial<TaskOverview> & { id: string };

export type TaskOverviewRead = {
  scope: string;
  changes: Record<string, TaskOverviewPatch | null>;
  bytes: number;
};

export type TaskOverviewState = {
  byId: Record<string, TaskOverview>;
  owners: Record<string, string[]>;
  reads: Record<string, TaskOverviewRead>;
  scope: string;
  generation: number;
  connectionGap: boolean;
};

export type TaskOverviewActions = {
  beginTaskOverviewRead: () => string;
  finishTaskOverviewRead: (readId: string) => void;
  retainTaskOverviews: (owner: string, tasks: TaskOverview[], readId?: string) => boolean;
  releaseTaskOverviews: (owner: string) => void;
  denyTaskOverviewAccess: () => void;
};

export type TaskOverviewSlice = TaskOverviewActions & { taskOverview: TaskOverviewState };

export function emptyTaskOverviewState(scope = "", generation = 0): TaskOverviewState {
  return { byId: {}, owners: {}, reads: {}, scope, generation, connectionGap: false };
}
