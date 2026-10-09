import type { useAppStoreApi } from "@/components/state-provider";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { compareUserSettingsRevisions } from "@/lib/settings/user-settings-revision";
import { requestUserSettingsUpdateWithRetry } from "@/lib/user-settings-sync";
import type { TaskColor } from "@/lib/task-colors";

type Store = ReturnType<typeof useAppStoreApi>;
type PendingColor = {
  operation: symbol;
  color: TaskColor | null;
  state: "queued" | "sent";
};
type SyncResult = {
  response: Awaited<ReturnType<typeof requestUserSettingsUpdateWithRetry>> | null;
  submittedIds: string[];
};
type QueuedRequest = {
  taskIds: string[];
  operation: symbol;
  color: TaskColor | null;
  resolve: (result: SyncResult) => void;
  reject: (reason: unknown) => void;
};

export type TaskColorResult = { saved: number; total: number };

function createColorSync(pending: Map<string, PendingColor>) {
  const requests: QueuedRequest[] = [];
  let requestRunning = false;

  function runNext() {
    if (requestRunning) return;
    let request = requests.shift();
    while (request) {
      const activeRequest = request;
      const submittedIds = activeRequest.taskIds.filter(
        (id) => pending.get(id)?.operation === activeRequest.operation,
      );
      if (submittedIds.length === 0) {
        activeRequest.resolve({ response: null, submittedIds });
        request = requests.shift();
        continue;
      }
      submittedIds.forEach((id) => {
        const entry = pending.get(id);
        if (entry?.operation === activeRequest.operation) {
          pending.set(id, { ...entry, state: "sent" });
        }
      });
      requestRunning = true;
      void requestUserSettingsUpdateWithRetry({
        sidebar_task_color_patch: {
          colors: Object.fromEntries(submittedIds.map((id) => [id, activeRequest.color])),
          if_missing: false,
        },
      })
        .then((response) => activeRequest.resolve({ response, submittedIds }))
        .catch((error: unknown) => activeRequest.reject(error))
        .finally(() => {
          requestRunning = false;
          runNext();
        });
      return;
    }
  }

  return (taskIds: string[], operation: symbol, color: TaskColor | null) =>
    new Promise<SyncResult>((resolve, reject) => {
      requests.push({ taskIds, operation, color, resolve, reject });
      runNext();
    });
}

function createColorMutation(store: Store) {
  let confirmed = { ...store.getState().userSettings.sidebarTaskColors };
  let revision = store.getState().userSettings.revision;
  let publishing = false;
  const pending = new Map<string, PendingColor>();
  const sync = createColorSync(pending);

  store.subscribe((state, previous) => {
    if (publishing || state.userSettings === previous.userSettings) return;
    const settings = state.userSettings;
    const order = compareUserSettingsRevisions(settings.revision, revision);
    if (order === 1 || (pending.size === 0 && order !== -1)) {
      const previousConfirmed = confirmed;
      confirmed = { ...settings.sidebarTaskColors };
      revision = settings.revision;
      if (order === 1) {
        pending.forEach((entry, id) => {
          const colorChanged =
            (settings.sidebarTaskColors[id] ?? null) !== (previousConfirmed[id] ?? null);
          if (entry.state === "sent" && colorChanged) pending.delete(id);
        });
        if (pending.size > 0) publish();
      }
    }
  });

  function publish() {
    const latest = store.getState().userSettings;
    const colors = { ...confirmed };
    pending.forEach(({ color }, id) => {
      colors[id] = color;
    });
    publishing = true;
    try {
      store
        .getState()
        .setUserSettings({ ...latest, sidebarTaskColors: colors, revision, loaded: true });
    } finally {
      publishing = false;
    }
  }

  function release(ids: string[], operation: symbol) {
    ids.forEach((id) => {
      if (pending.get(id)?.operation === operation) pending.delete(id);
    });
  }

  return async (taskIds: string[], color: TaskColor | null): Promise<TaskColorResult> => {
    const ids = [...new Set(taskIds.filter(Boolean))];
    const operation = Symbol();
    ids.forEach((id) => pending.set(id, { operation, color, state: "queued" }));
    if (ids.length === 0) return { saved: 0, total: 0 };
    publish();
    let saved = 0;
    try {
      for (let offset = 0; offset < ids.length; offset += 500) {
        const chunk = ids.slice(offset, offset + 500);
        const { response, submittedIds } = await sync(chunk, operation, color);
        if (!response) {
          saved += submittedIds.length;
          continue;
        }
        const mapped = mapUserSettingsResponse(response, store.getState().userSettings);
        const order = compareUserSettingsRevisions(mapped.revision, revision);
        if (order === 1 || order === 0) {
          confirmed = { ...mapped.sidebarTaskColors };
          revision = mapped.revision;
        }
        saved += submittedIds.length;
        release(submittedIds, operation);
        publish();
      }
    } catch {
      release(ids, operation);
      publish();
    }
    return { saved, total: ids.length };
  };
}

const mutations = new WeakMap<Store, ReturnType<typeof createColorMutation>>();

export function getTaskColorMutation(store: Store) {
  let mutation = mutations.get(store);
  if (!mutation) {
    mutation = createColorMutation(store);
    mutations.set(store, mutation);
  }
  return mutation;
}
