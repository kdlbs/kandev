"use client";

import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { useCoordinatorList } from "./use-coordinator-list";

export type ResolvedCoordinatorState =
  | { status: "loading" }
  | { status: "list-error"; retry: () => void }
  | { status: "no-coordinator" }
  | { status: "unknown-coordinator"; retry: () => void }
  | { status: "redirect"; target: Coordinator }
  | { status: "ready"; coordinator: Coordinator; coordinators: Coordinator[] };

/**
 * Resolves which coordinator a coordinator route should show
 * (docs/specs/coordinator/system-design/needs-you.md#routes-and-sidebar,
 * AC-COORDINATOR-NEEDS-YOU-006.4). `coordinatorId` is `null` for the generic
 * `/workspaces/:id/coordinator` route, which resolves to the first
 * coordinator by list order.
 */
export function useResolvedCoordinator(
  workspaceId: string | null,
  coordinatorId: string | null,
): ResolvedCoordinatorState {
  const { coordinators, error, retry } = useCoordinatorList(workspaceId);

  if (error) return { status: "list-error", retry };
  if (coordinators === undefined) return { status: "loading" };
  if (coordinators.length === 0) return { status: "no-coordinator" };
  if (coordinatorId === null) return { status: "redirect", target: coordinators[0] };

  const match = coordinators.find((c) => c.id === coordinatorId);
  if (!match) return { status: "unknown-coordinator", retry };

  return { status: "ready", coordinator: match, coordinators };
}
