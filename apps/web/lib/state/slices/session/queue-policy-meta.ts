import type { QueueMeta, QueueStatus } from "./types";

const POLICIES = [
  {
    available: "autoMergeAvailable",
    enabled: "autoMergeEnabled",
    source: "autoMergeSource",
    revision: "autoMergeRevision",
    wireAvailable: "auto_merge_available",
    wireEnabled: "auto_merge_enabled",
    wireSource: "auto_merge_source",
    wireRevision: "auto_merge_revision",
  },
] as const;

type PolicyFields = (typeof POLICIES)[number];
type PolicyStatus = Partial<
  Pick<QueueStatus, PolicyFields["wireAvailable" | "wireEnabled" | "wireSource" | "wireRevision"]>
>;

/** Omission is not an authoritative policy value, including on recovery reads. */
export function queuePolicyStatusMeta(status: PolicyStatus): Partial<QueueMeta> {
  const result: Partial<QueueMeta> = {};
  for (const keys of POLICIES) {
    if (status[keys.wireAvailable] !== undefined)
      result[keys.available] = status[keys.wireAvailable];
    if (status[keys.wireEnabled] !== undefined) result[keys.enabled] = status[keys.wireEnabled];
    if (status[keys.wireSource] !== undefined) result[keys.source] = status[keys.wireSource];
    if (status[keys.wireRevision] !== undefined) result[keys.revision] = status[keys.wireRevision];
  }
  return result;
}

function shouldPreserveQueuePolicy(
  previous: QueueMeta,
  incoming: Partial<QueueMeta>,
  keys: PolicyFields,
): boolean {
  const previousSource = previous[keys.source];
  const nextSource = incoming[keys.source];
  if (previousSource === "session" && nextSource === "global") return true;
  const previousRevision = previous[keys.revision];
  const nextRevision = incoming[keys.revision];
  return (
    previousSource === nextSource &&
    previousRevision !== undefined &&
    nextRevision !== undefined &&
    nextRevision < previousRevision
  );
}

/** Each preference has its own irreversible session source and revision. */
export function mergeQueuePolicyMeta(
  previous: QueueMeta | undefined,
  incoming: Partial<QueueMeta>,
): Partial<QueueMeta> {
  const result: Partial<QueueMeta> = {};
  const old =
    previous?.sessionIncarnationId === incoming.sessionIncarnationId ? previous : undefined;
  for (const keys of POLICIES) {
    if (old && shouldPreserveQueuePolicy(old, incoming, keys)) {
      result[keys.available] = old[keys.available];
      result[keys.enabled] = old[keys.enabled];
      result[keys.source] = old[keys.source];
      result[keys.revision] = old[keys.revision];
      continue;
    }
    result[keys.available] = incoming[keys.available] ?? old?.[keys.available];
    result[keys.enabled] = incoming[keys.enabled] ?? old?.[keys.enabled];
    result[keys.source] = incoming[keys.source] ?? old?.[keys.source];
    result[keys.revision] = incoming[keys.revision] ?? old?.[keys.revision];
  }
  return result;
}
