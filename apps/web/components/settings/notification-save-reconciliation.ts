import type { NotificationProvider } from "@/lib/types/http";
import type { NotificationsState } from "./notifications-settings-actions";

function reconcileEvents(
  current: string[] = [],
  submitted: string[] = [],
  accepted: string[] = [],
) {
  const removed = new Set(submitted.filter((event) => !current.includes(event)));
  const added = current.filter((event) => !submitted.includes(event));
  return [...new Set([...accepted.filter((event) => !removed.has(event)), ...added])];
}

function reconcileProvider(
  current: NotificationProvider,
  submitted: NotificationProvider,
  accepted: NotificationProvider,
): NotificationProvider {
  const urlsChanged =
    JSON.stringify(current.config?.urls) !== JSON.stringify(submitted.config?.urls);
  return {
    ...accepted,
    enabled: current.enabled === submitted.enabled ? accepted.enabled : current.enabled,
    name: current.name === submitted.name ? accepted.name : current.name,
    events: reconcileEvents(current.events, submitted.events, accepted.events),
    config: urlsChanged ? { ...accepted.config, urls: current.config?.urls } : accepted.config,
  };
}

function reconcileBuffers(
  current: Record<string, string>,
  submitted: Record<string, string>,
  accepted: Record<string, string>,
) {
  const next = { ...current };
  for (const [id, value] of Object.entries(accepted)) {
    if (current[id] === submitted[id]) next[id] = value;
  }
  return next;
}

type Acknowledgement = {
  submitted: NotificationsState;
  current: NotificationsState;
  accepted: NotificationProvider[];
  edits: { urls: Record<string, string>; names: Record<string, string> };
  created: NotificationProvider | null;
  formIdentity: object;
};

export function acknowledgeNotificationSave({
  submitted,
  current,
  accepted,
  edits,
  created,
  formIdentity,
}: Acknowledgement) {
  const submittedById = new Map(submitted.providers.map((provider) => [provider.id, provider]));
  const acceptedById = new Map(accepted.map((provider) => [provider.id, provider]));
  submitted.setProviders((providers) => {
    const next = providers
      .filter((provider) => !submitted.pendingDeletes.has(provider.id))
      .map((provider) => {
        const before = submittedById.get(provider.id);
        const after = acceptedById.get(provider.id);
        return before && after ? reconcileProvider(provider, before, after) : provider;
      });
    return created ? [...next, created] : next;
  });
  submitted.setBaselineProviders(accepted);
  submitted.setAppriseEdits((buffers) =>
    reconcileBuffers(buffers, submitted.appriseEdits, edits.urls),
  );
  submitted.setAppriseNameEdits((buffers) =>
    reconcileBuffers(buffers, submitted.appriseNameEdits, edits.names),
  );
  submitted.setPendingDeletes(
    (ids) => new Set([...ids].filter((id) => !submitted.pendingDeletes.has(id))),
  );

  // Form identity distinguishes a replacement with identical text from the submitted form.
  if (current.appriseFormIdentity.current !== formIdentity) return;
  if (created) {
    if (
      current.appriseName !== submitted.appriseName ||
      current.appriseUrls !== submitted.appriseUrls
    )
      return;
    submitted.setAppriseName("");
    submitted.setAppriseUrls("");
    submitted.setShowAppriseForm(false);
    submitted.setActiveAppriseId(null);
  } else if (current.appriseFormMode === "edit" && current.activeAppriseId) {
    const id = current.activeAppriseId;
    if (current.appriseName === submitted.appriseName)
      submitted.setAppriseName(edits.names[id] ?? current.appriseName);
    if (current.appriseUrls === submitted.appriseUrls)
      submitted.setAppriseUrls(edits.urls[id] ?? current.appriseUrls);
  }
}
