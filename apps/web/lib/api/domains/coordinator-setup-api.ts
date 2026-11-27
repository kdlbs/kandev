import type { ApiRequestOptions } from "@/lib/api/client";
import type * as Projects from "./coordinator-projects-types";
import {
  mutate,
  workspacePath,
  type ControlAction,
  type ControlSetting,
  type Coordinator,
  type CreateCoordinatorRequest,
} from "./coordinator-api";

// Mirrors internal/coordinator/setup.go's body: one request creates the
// coordinator with its policy, Watches and optional goal, or none of them.
export type SetupCoordinatorRequest = Required<CreateCoordinatorRequest> & {
  watches: { scope: "all" | "selected"; workflow_ids?: string[] };
  projects?: Projects.ProjectsRequest;
  policy: { actions: Record<ControlAction, ControlSetting> };
  goal?: {
    name: string;
    due_on: string | null;
    criteria: Array<{ text: string }>;
  };
};

export function setupCoordinator(
  workspaceId: string,
  req: SetupCoordinatorRequest,
  options?: ApiRequestOptions,
): Promise<Coordinator> {
  return mutate<Coordinator>(
    workspacePath(workspaceId, "/coordinators/setup"),
    "POST",
    req,
    options,
  );
}
