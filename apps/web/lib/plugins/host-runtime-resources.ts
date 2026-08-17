import { reviewItemId } from "@/components/task/review-selection";
import { reviewPanelId } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type {
  PluginHostApi,
  PluginModalHandle,
  PluginStorageApi,
  PluginTaskReviewOptions,
  PluginToastApi,
} from "./types";

function pluginLoadAbortError(): DOMException {
  return new DOMException("Plugin load is no longer active", "AbortError");
}

/** Every side effect owned by one successful or in-flight load generation. */
export class PluginLoadResources {
  private readonly controller = new AbortController();
  private readonly cleanups = new Set<() => void>();
  private active = true;

  constructor(private readonly pluginId: string) {}

  /** Registers an idempotent cleanup and returns it as the public disposer. */
  track(cleanup: () => void): () => void {
    let pending = true;
    const dispose = () => {
      if (!pending) return;
      pending = false;
      this.cleanups.delete(dispose);
      try {
        cleanup();
      } catch (error) {
        console.error(`[plugins] error cleaning up resource for "${this.pluginId}"`, error);
      }
    };
    if (this.active) this.cleanups.add(dispose);
    else dispose();
    return dispose;
  }

  /** Runs one request under both the caller's signal and this load generation. */
  runRequest<T>(
    callerSignal: AbortSignal | null | undefined,
    operation: (signal: AbortSignal) => Promise<T>,
  ): Promise<T> {
    if (!this.active || this.controller.signal.aborted || callerSignal?.aborted) {
      return Promise.reject(pluginLoadAbortError());
    }

    const requestController = new AbortController();
    const abort = () => requestController.abort(pluginLoadAbortError());
    this.controller.signal.addEventListener("abort", abort, { once: true });
    callerSignal?.addEventListener("abort", abort, { once: true });

    let request: Promise<T>;
    try {
      request = operation(requestController.signal);
    } catch (error) {
      request = Promise.reject(error as Error);
    }
    return request.finally(() => {
      this.controller.signal.removeEventListener("abort", abort);
      callerSignal?.removeEventListener("abort", abort);
    });
  }

  /** Revokes the generation in abort-first order, then disposes owned UI/listeners. */
  revoke(): void {
    if (!this.active) return;
    this.active = false;
    this.controller.abort(pluginLoadAbortError());
    for (const cleanup of [...this.cleanups]) cleanup();
  }
}

/** Fences and owns host-side effects for one load generation. */
export function generationFencedHost(
  host: PluginHostApi,
  isCurrent: () => boolean,
  resources: PluginLoadResources,
): PluginHostApi {
  const inertHandle = (): ReturnType<PluginHostApi["openModal"]> => ({ close: () => {} });
  const setState = ((...args: unknown[]) => {
    if (isCurrent()) {
      (host.store.setState as unknown as (...forwarded: unknown[]) => void)(...args);
    }
  }) as PluginHostApi["store"]["setState"];
  const subscribe: PluginHostApi["store"]["subscribe"] = (listener) => {
    if (!isCurrent()) return () => {};
    return resources.track(
      host.store.subscribe((state, previousState) => {
        if (isCurrent()) listener(state, previousState);
      }),
    );
  };
  const fetch: PluginHostApi["api"]["fetch"] = (path, init) => {
    if (!isCurrent()) return Promise.reject(pluginLoadAbortError());
    return resources.runRequest(init?.signal, (signal) =>
      host.api.fetch(path, { ...init, signal }),
    );
  };
  const invokeAction: PluginHostApi["api"]["invokeAction"] = (key, input, options) => {
    if (!isCurrent()) return Promise.reject(pluginLoadAbortError());
    return resources.runRequest(options?.signal, (signal) =>
      host.api.invokeAction(key, input, { ...options, signal }),
    );
  };

  return {
    pluginId: host.pluginId,
    React: host.React,
    jsx: host.jsx,
    store: { ...host.store, setState, subscribe },
    context: generationFencedContext(host, isCurrent, resources),
    api: {
      fetch,
      invokeAction,
      get baseUrl() {
        return host.api.baseUrl;
      },
    },
    ui: host.ui,
    i18n: host.i18n,
    useResponsiveBreakpoint: host.useResponsiveBreakpoint,
    get theme() {
      return host.theme;
    },
    onThemeChange: (listener) => {
      if (!isCurrent()) return () => {};
      return resources.track(
        host.onThemeChange((theme) => {
          if (isCurrent()) listener(theme);
        }),
      );
    },
    navigate: (...args) => {
      if (isCurrent()) host.navigate(...args);
    },
    openModal: (options) =>
      isCurrent() ? trackModalHandle(resources, host.openModal(options)) : inertHandle(),
    openTaskLinkDialog: (options) =>
      isCurrent() ? trackModalHandle(resources, host.openTaskLinkDialog(options)) : inertHandle(),
    openTaskReview: (options) => {
      if (isCurrent()) openTaskReviewAndTrack(host, resources, options);
    },
    toast: generationFencedToast(host.toast, isCurrent, resources),
    utils: host.utils,
    storage: generationFencedStorage(host.storage, isCurrent, resources),
    useSettingsSaveContributor: host.useSettingsSaveContributor,
    setIntegrationEnabled: (integrationId, workspaceId, enabled) => {
      if (isCurrent()) host.setIntegrationEnabled(integrationId, workspaceId, enabled);
    },
  };
}

function generationFencedContext(
  host: PluginHostApi,
  isCurrent: () => boolean,
  resources: PluginLoadResources,
): PluginHostApi["context"] {
  return {
    getActiveWorkspaceId: () => (isCurrent() ? host.context.getActiveWorkspaceId() : undefined),
    subscribeActiveWorkspace: (listener) =>
      generationFencedSubscription(
        host.context.subscribeActiveWorkspace,
        host.context,
        isCurrent,
        resources,
        listener,
      ),
    getWorkspaceIds: () => (isCurrent() ? host.context.getWorkspaceIds() : []),
    subscribeWorkspaces: (listener) =>
      generationFencedSubscription(
        host.context.subscribeWorkspaces,
        host.context,
        isCurrent,
        resources,
        listener,
      ),
    getTaskCreationContext: (workspaceId) =>
      isCurrent() ? host.context.getTaskCreationContext(workspaceId) : null,
    subscribeTaskCreationContext: (workspaceId, listener) => {
      if (!isCurrent()) return () => {};
      return resources.track(
        host.context.subscribeTaskCreationContext(workspaceId, (context) => {
          if (isCurrent()) listener(context);
        }),
      );
    },
    resolveRepositoryId: (identity) =>
      isCurrent() ? host.context.resolveRepositoryId(identity) : undefined,
  };
}

function generationFencedSubscription<Value>(
  subscribe: (listener: (value: Value) => void) => () => void,
  context: unknown,
  isCurrent: () => boolean,
  resources: PluginLoadResources,
  listener: (value: Value) => void,
): () => void {
  if (!isCurrent()) return () => {};
  return resources.track(
    subscribe.call(context, (value) => {
      if (isCurrent()) listener(value);
    }),
  );
}

function trackModalHandle(
  resources: PluginLoadResources,
  handle: PluginModalHandle,
): PluginModalHandle {
  return { close: resources.track(() => handle.close()) };
}

function generationFencedStorage(
  storage: PluginStorageApi,
  isCurrent: () => boolean,
  resources: PluginLoadResources,
): PluginStorageApi {
  const unavailable = <T>(): Promise<T> => Promise.reject(pluginLoadAbortError());
  return {
    get: (scope, scopeId, key, options) =>
      isCurrent()
        ? resources.runRequest(options?.signal, (signal) =>
            storage.get(scope, scopeId, key, { ...options, signal }),
          )
        : unavailable(),
    set: (scope, scopeId, key, value, options) =>
      isCurrent()
        ? resources.runRequest(options?.signal, (signal) =>
            storage.set(scope, scopeId, key, value, { ...options, signal }),
          )
        : unavailable(),
    delete: (scope, scopeId, key, options) =>
      isCurrent()
        ? resources.runRequest(options?.signal, (signal) =>
            storage.delete(scope, scopeId, key, { ...options, signal }),
          )
        : unavailable(),
    list: (scope, scopeId, options) =>
      isCurrent()
        ? resources.runRequest(options?.signal, (signal) =>
            storage.list(scope, scopeId, { ...options, signal }),
          )
        : unavailable(),
    subscribe: (filter, handler) => {
      if (!isCurrent()) return () => {};
      return resources.track(
        storage.subscribe(filter, (change) => {
          if (isCurrent()) handler(change);
        }),
      );
    },
  };
}

function generationFencedToast(
  toast: PluginToastApi,
  isCurrent: () => boolean,
  resources: PluginLoadResources,
): PluginToastApi {
  const activeToasts = new Map<string | number, () => void>();
  const show = (
    method: "default" | "success" | "error" | "warning" | "info",
    message: string,
    options?: Record<string, unknown>,
  ): string | number => {
    if (!isCurrent()) return 0;
    const id = method === "default" ? toast(message, options) : toast[method](message, options);
    const dispose = resources.track(() => {
      activeToasts.delete(id);
      toast.dismiss(id);
    });
    activeToasts.set(id, dispose);
    return id;
  };
  const fenced = ((message: string, options?: Record<string, unknown>) =>
    show("default", message, options)) as PluginToastApi;
  fenced.success = (message, options) => show("success", message, options);
  fenced.error = (message, options) => show("error", message, options);
  fenced.warning = (message, options) => show("warning", message, options);
  fenced.info = (message, options) => show("info", message, options);
  fenced.dismiss = (id) => {
    if (id === undefined) {
      for (const dispose of [...activeToasts.values()]) dispose();
      return;
    }
    activeToasts.get(id)?.();
  };
  return fenced;
}

function openTaskReviewAndTrack(
  host: PluginHostApi,
  resources: PluginLoadResources,
  options: PluginTaskReviewOptions,
): void {
  if (options.presentation === "desktop") {
    const id = reviewPanelId(options);
    const existed = Boolean(useDockviewStore.getState().api?.getPanel(id));
    host.openTaskReview(options);
    if (!existed && useDockviewStore.getState().api?.getPanel(id)) {
      resources.track(() => {
        const api = useDockviewStore.getState().api;
        const panel = api?.getPanel(id);
        if (api && panel) api.removePanel(panel);
      });
    }
    return;
  }

  const before = host.store.getState();
  const previousReview = before.mobileSession?.reviewItemIdBySessionId?.[options.sessionId];
  const previousPanel = before.mobileSession?.activePanelBySessionId?.[options.sessionId];
  const openedReview = reviewItemId(options);
  host.openTaskReview(options);
  if (typeof before.setMobileSessionReview !== "function") return;
  resources.track(() => {
    const current = host.store.getState();
    if (current.mobileSession?.reviewItemIdBySessionId?.[options.sessionId] !== openedReview)
      return;
    current.setMobileSessionReview(options.sessionId, previousReview ?? null);
    if (previousPanel && typeof current.setMobileSessionPanel === "function") {
      current.setMobileSessionPanel(options.sessionId, previousPanel);
    }
  });
}
