import type { EntityReference } from "@/lib/types/entity-reference";
import { entityReferenceMarkdown } from "@/lib/entity-references/message-references";

export function queueReferenceSelectionsChanged(
  previous: readonly EntityReference[],
  current: readonly EntityReference[],
): boolean {
  const identity = (references: readonly EntityReference[]) =>
    references.map((reference) => [
      reference.ref,
      reference.selection_id,
      reference.association_mode,
    ]);
  return JSON.stringify(identity(previous)) !== JSON.stringify(identity(current));
}

export function selectQueueReference(
  value: string,
  references: readonly EntityReference[],
  selection: EntityReference,
) {
  const token = entityReferenceMarkdown(selection);
  return {
    value: value.includes(token) ? value : `${value.trimEnd()}${value.trim() ? " " : ""}${token}`,
    references: [...references.filter((reference) => reference.ref !== selection.ref), selection],
  };
}

export function removeQueueReference(
  value: string,
  references: readonly EntityReference[],
  ref: string,
) {
  const reference = references.find((item) => item.ref === ref);
  return {
    value: reference ? value.split(entityReferenceMarkdown(reference)).join("").trim() : value,
    references: references.filter((item) => item.ref !== ref),
  };
}
