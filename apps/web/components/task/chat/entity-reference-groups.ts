import type { EntityReference, EntityReferenceSearchGroup } from "@/lib/types/entity-reference";

export function visibleEntityReferenceGroups(
  groups: readonly EntityReferenceSearchGroup[],
): EntityReferenceSearchGroup[] {
  return groups.filter(
    (group) =>
      group.source !== "kandev_tasks" &&
      group.status !== "not_configured" &&
      group.status !== "unsupported_scope" &&
      (group.status !== "ok" || group.results.length > 0),
  );
}

export function selectableEntityReferences(
  groups: readonly EntityReferenceSearchGroup[],
): EntityReference[] {
  return visibleEntityReferenceGroups(groups).flatMap((group) =>
    group.status === "ok" ? group.results : [],
  );
}
