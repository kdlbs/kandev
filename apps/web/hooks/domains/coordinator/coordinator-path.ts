const COORDINATOR_PATH = /^\/workspaces\/[^/]+\/coordinator\/([^/]+)(?:\/queue)?\/?$/;

/** The coordinator id of a Needs you or Queue path, or null for any other path. */
export function coordinatorIdFromPath(pathname: string): string | null {
  const match = COORDINATOR_PATH.exec(pathname);
  return match ? decodeURIComponent(match[1]) : null;
}
