import { deleteTaskPR } from "@/lib/api/domains/github-api";
import type { AppState } from "@/lib/state/store";

type UnlinkTaskPRAssociationOptions = {
  associationId: string;
  taskId: string | null;
  workspaceId: string | null;
  workspaceContextGeneration: number;
  isWorkspaceContextCurrent: () => boolean;
  removeTaskPR: AppState["removeTaskPR"];
  invalidateSync: () => void;
};

export async function unlinkTaskPRAssociation({
  associationId,
  taskId,
  workspaceId,
  workspaceContextGeneration,
  isWorkspaceContextCurrent,
  removeTaskPR,
  invalidateSync,
}: UnlinkTaskPRAssociationOptions): Promise<void> {
  if (!taskId || !workspaceId) throw new Error("No active workspace is selected.");

  await deleteTaskPR(associationId, workspaceId);
  if (!isWorkspaceContextCurrent()) return;
  removeTaskPR(taskId, associationId, { workspaceId, workspaceContextGeneration });
  invalidateSync();
}
