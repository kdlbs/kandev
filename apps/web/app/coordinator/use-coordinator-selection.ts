import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSearchParams, useRouter } from "@/lib/routing/client-router";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { coordinatorHref } from "@/lib/api/domains/orchestration-api";

const selections = new Map<string, string>();
let selectionOwner: string | undefined;

export function resolveCoordinatorSelection(
  assignments: { id: string }[],
  requested: string | null,
  remembered?: string,
): string {
  if (requested !== null) return assignments.some((item) => item.id === requested) ? requested : "";
  if (
    remembered !== undefined &&
    (remembered === "" || assignments.some((item) => item.id === remembered))
  )
    return remembered;
  return assignments.length === 1 ? assignments[0].id : "";
}

export function useCoordinatorSelection(catalog: CoordinatorWorkspace) {
  const params = useSearchParams();
  const router = useRouter();
  const owner = useAppStore((s) => s.auth.user?.id);
  if (owner !== selectionOwner) {
    selections.clear();
    selectionOwner = owner;
  }
  const workspace = catalog.workspace.id;
  const requested = params.get("orchestratorId");
  const selected = resolveCoordinatorSelection(
    catalog.assignments,
    requested,
    selections.get(workspace),
  );
  useEffect(() => {
    if (selected || requested === "") selections.set(workspace, selected);
  }, [workspace, requested, selected]);
  const choose = (id: string) => {
    const value = id === "none" ? "" : id;
    selections.set(workspace, value);
    router.replace(coordinatorHref(workspace, value), { scroll: false });
  };
  return { requested, selected, choose };
}
