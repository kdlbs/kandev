import type { StoragePendingAction } from "@/hooks/domains/system/use-storage-maintenance";

type Translate = (key: string) => string;

/**
 * Why the storage maintenance actions (analyze, cleanup, quarantine
 * restore/purge) are unavailable, or `undefined` when they are.
 *
 * The admin gate wins over the transient pending reason: those routes are
 * mounted on the admin group, so for a member they are permanently
 * unavailable rather than momentarily busy.
 */
export function storageActionDisabledReason(
  t: Translate,
  action: StoragePendingAction,
  isAdmin: boolean,
): string | undefined {
  if (!isAdmin) return t("system:storageAdminOnly");
  if (action) return t("system:storageActionPending");
  return undefined;
}

/** Why the storage policy form cannot be edited or saved, if it cannot be. */
export function policyBlockedReason(
  t: Translate,
  action: StoragePendingAction,
  loading: boolean,
  isAdmin: boolean,
): string | undefined {
  if (!isAdmin) return t("system:storageAdminOnly");
  if (action === "adopt") return t("system:storageAdoptionPending");
  if (loading) return t("system:storagePolicyLoadingBlock");
  return undefined;
}

/**
 * Whether the storage policy form's controls are locked: either an action
 * that rewrites the policy is already in flight, or the caller may not edit
 * it at all (see `policyBlockedReason`).
 */
export function policyControlsPending(
  action: StoragePendingAction,
  readOnlyReason: string | undefined,
): boolean {
  return action === "save" || action === "adopt" || Boolean(readOnlyReason);
}
