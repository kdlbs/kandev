import { listAgentUpdateStatuses } from "@/lib/api/domains/agent-update-api";
import type { SettingsSlice } from "@/lib/state/slices/settings/types";

type RuntimeStatusState = Pick<
  SettingsSlice,
  "agentRuntimeUpdates" | "setAgentRuntimeUpdateStatuses" | "setAgentRuntimeUpdateLoading"
>;
export type RuntimeStatusStore = { getState(): RuntimeStatusState };
type Request = { promise: Promise<boolean>; queued?: Promise<boolean> };
const requests = new WeakMap<RuntimeStatusStore, Request>();
const CLIENT_STATUS_TTL = 60_000;

// Requests belong to the store so an unmounted initiator cannot abandon loading.
export function refreshRuntimeUpdateStatuses(
  store: RuntimeStatusStore,
  force = false,
): Promise<boolean> {
  const pending = requests.get(store);
  if (pending) {
    if (!force) return pending.promise;
    pending.queued ??= pending.promise.then(() => refreshRuntimeUpdateStatuses(store, true));
    return pending.queued;
  }
  const { checkedAt } = store.getState().agentRuntimeUpdates;
  if (!force && checkedAt > 0 && Date.now() - checkedAt < CLIENT_STATUS_TTL)
    return Promise.resolve(true);
  store.getState().setAgentRuntimeUpdateLoading(true);
  const request: Request = {
    promise: listAgentUpdateStatuses({ cache: "no-store" })
      .then((response) => {
        store.getState().setAgentRuntimeUpdateStatuses(response.statuses, Date.now());
        return true;
      })
      .catch(() => false)
      .finally(() => {
        store.getState().setAgentRuntimeUpdateLoading(false);
        if (requests.get(store) === request) requests.delete(store);
      }),
  };
  requests.set(store, request);
  return request.promise;
}
