import { matchDouble } from "@/lib/routing/path";

const COORDINATOR_PATH = /^\/workspaces\/([^/]+)\/coordinator\/([^/]+)(?:\/queue)?\/?$/;

/** The coordinator id of a Needs you or Queue path, or null for any other path. */
export function coordinatorIdFromPath(pathname: string): string | null {
  return matchDouble(pathname, COORDINATOR_PATH)?.[1] ?? null;
}
