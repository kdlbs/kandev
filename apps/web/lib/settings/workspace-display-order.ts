type WorkspaceDisplayItem = { id: string };

/**
 * Puts the active workspace first without changing the order of any other
 * workspace. Unknown or missing active IDs leave the input order unchanged.
 */
export function orderWorkspacesForDisplay<S extends WorkspaceDisplayItem>(
  workspaces: ReadonlyArray<S>,
  activeWorkspaceId?: string | null,
): S[] {
  const activeIndex =
    activeWorkspaceId == null
      ? -1
      : workspaces.findIndex((workspace) => workspace.id === activeWorkspaceId);

  if (activeIndex <= 0) return [...workspaces];

  return [
    workspaces[activeIndex],
    ...workspaces.slice(0, activeIndex),
    ...workspaces.slice(activeIndex + 1),
  ];
}
