import type { NotificationProvider } from "@/lib/types/http";
import type { NotificationsState } from "./notifications-settings-actions";
import { webhookHeadersSignature } from "./notifications-settings-webhook-helpers";

function reconcileEvents(
  current: string[] | null = [],
  submitted: string[] | null = [],
  accepted: string[] | null = [],
) {
  // Older payloads serialize a provider with no enabled subscriptions as
  // `events: null`, so normalize before filtering.
  const currentEvents = current ?? [];
  const submittedEvents = submitted ?? [];
  const acceptedEvents = accepted ?? [];
  const removed = new Set(submittedEvents.filter((event) => !currentEvents.includes(event)));
  const added = currentEvents.filter((event) => !submittedEvents.includes(event));
  return [...new Set([...acceptedEvents.filter((event) => !removed.has(event)), ...added])];
}

function reconcileProvider(
  current: NotificationProvider,
  submitted: NotificationProvider,
  accepted: NotificationProvider,
): NotificationProvider {
  const urlsChanged =
    JSON.stringify(current.config?.urls) !== JSON.stringify(submitted.config?.urls);
  const webhookChanged =
    current.type === "webhook" &&
    JSON.stringify(current.config) !== JSON.stringify(submitted.config);
  let config = accepted.config;
  if (urlsChanged) {
    config = { ...accepted.config, urls: current.config?.urls };
  } else if (webhookChanged) {
    config = current.config;
  }
  return {
    ...accepted,
    enabled: current.enabled === submitted.enabled ? accepted.enabled : current.enabled,
    name: current.name === submitted.name ? accepted.name : current.name,
    events: reconcileEvents(current.events, submitted.events, accepted.events),
    config,
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
  formIdentity: { apprise: object; webhook: object };
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
  submitted.setWebhookHeaderDrafts((drafts) => {
    const next = { ...drafts };
    for (const providerId of submitted.pendingDeletes) delete next[providerId];
    for (const [providerId, submittedHeaders] of Object.entries(submitted.webhookHeaderDrafts)) {
      if (drafts[providerId] === submittedHeaders) delete next[providerId];
    }
    return next;
  });
  submitted.setPendingDeletes(
    (ids) => new Set([...ids].filter((id) => !submitted.pendingDeletes.has(id))),
  );

  // Form identity distinguishes a replacement with identical text from the submitted form.
  if (created?.type === "webhook") {
    acknowledgeCreatedWebhook(submitted, current, formIdentity);
  } else if (created) {
    acknowledgeCreatedApprise(submitted, current, formIdentity);
  } else if (current.appriseFormMode === "edit" && current.activeAppriseId) {
    acknowledgeAppriseEdit(submitted, current, edits, formIdentity);
  }
}

function acknowledgeCreatedWebhook(
  submitted: NotificationsState,
  current: NotificationsState,
  formIdentity: Acknowledgement["formIdentity"],
) {
  if (current.webhookFormIdentity.current !== formIdentity.webhook) return;
  if (
    current.webhookUrl !== submitted.webhookUrl ||
    current.webhookName !== submitted.webhookName ||
    current.webhookSecret !== submitted.webhookSecret ||
    webhookHeadersSignature(current.webhookHeaders) !==
      webhookHeadersSignature(submitted.webhookHeaders)
  )
    return;
  submitted.setWebhookName("");
  submitted.setWebhookUrl("");
  submitted.setWebhookHeaders("");
  submitted.setWebhookSecret("");
  submitted.setShowWebhookForm(false);
  submitted.setActiveWebhookId(null);
  submitted.webhookFormRef.current = { open: false, mode: "create" };
}

function acknowledgeCreatedApprise(
  submitted: NotificationsState,
  current: NotificationsState,
  formIdentity: Acknowledgement["formIdentity"],
) {
  if (current.appriseFormIdentity.current !== formIdentity.apprise) return;
  if (
    current.appriseName !== submitted.appriseName ||
    current.appriseUrls !== submitted.appriseUrls
  )
    return;
  submitted.setAppriseName("");
  submitted.setAppriseUrls("");
  submitted.setShowAppriseForm(false);
  submitted.setActiveAppriseId(null);
  submitted.appriseFormRef.current = { open: false, mode: "create" };
}

function acknowledgeAppriseEdit(
  submitted: NotificationsState,
  current: NotificationsState,
  edits: Acknowledgement["edits"],
  formIdentity: Acknowledgement["formIdentity"],
) {
  if (current.appriseFormIdentity.current !== formIdentity.apprise) return;
  const id = current.activeAppriseId;
  if (!id) return;
  if (current.appriseName === submitted.appriseName)
    submitted.setAppriseName(edits.names[id] ?? current.appriseName);
  if (current.appriseUrls === submitted.appriseUrls)
    submitted.setAppriseUrls(edits.urls[id] ?? current.appriseUrls);
}
