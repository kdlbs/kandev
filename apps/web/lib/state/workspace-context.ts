export type WorkspaceContextState = {
  workspaces: { activeId: string | null };
  workspaceContextGeneration: number;
};

export function isCurrentWorkspaceContext(
  state: WorkspaceContextState,
  requestedWorkspaceId: string | null,
  requestedGeneration: number,
): boolean {
  return (
    state.workspaces.activeId === requestedWorkspaceId &&
    state.workspaceContextGeneration === requestedGeneration
  );
}
