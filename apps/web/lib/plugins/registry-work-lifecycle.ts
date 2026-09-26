type PluginOperation<T> = (signal: AbortSignal) => Promise<T>;

function abortError(): DOMException {
  return new DOMException("Plugin work was aborted", "AbortError");
}

/** Owns cancellation and subscription fencing for one registry instance. */
export class PluginWorkLifecycle {
  private abortControllersByPlugin = new Map<string, Set<AbortController>>();
  private reviewUnsubscribersByPlugin = new Map<string, Set<() => void>>();

  run<T>(pluginId: string, sourceSignal: AbortSignal, operation: PluginOperation<T>): Promise<T> {
    const controller = new AbortController();
    const controllers = this.abortControllersByPlugin.get(pluginId) ?? new Set<AbortController>();
    this.abortControllersByPlugin.set(pluginId, controllers);
    controllers.add(controller);
    const forwardAbort = () => controller.abort();
    if (sourceSignal.aborted) forwardAbort();
    else sourceSignal.addEventListener("abort", forwardAbort, { once: true });

    const aborted = new Promise<never>((_resolve, reject) => {
      if (controller.signal.aborted) reject(abortError());
      else controller.signal.addEventListener("abort", () => reject(abortError()), { once: true });
    });
    const work = Promise.resolve()
      .then(() => {
        if (controller.signal.aborted) throw abortError();
        return operation(controller.signal);
      })
      .then((value) => {
        if (controller.signal.aborted) throw abortError();
        return value;
      });

    return Promise.race([work, aborted]).finally(() => {
      sourceSignal.removeEventListener("abort", forwardAbort);
      controllers.delete(controller);
      if (controllers.size === 0 && this.abortControllersByPlugin.get(pluginId) === controllers) {
        this.abortControllersByPlugin.delete(pluginId);
      }
    });
  }

  trackSubscription(pluginId: string, unsubscribe: () => void): () => void {
    const unsubscribers = this.reviewUnsubscribersByPlugin.get(pluginId) ?? new Set<() => void>();
    this.reviewUnsubscribersByPlugin.set(pluginId, unsubscribers);
    let closed = false;
    const trackedUnsubscribe = () => {
      if (closed) return;
      closed = true;
      unsubscribers.delete(trackedUnsubscribe);
      if (unsubscribers.size === 0) this.reviewUnsubscribersByPlugin.delete(pluginId);
      unsubscribe();
    };
    unsubscribers.add(trackedUnsubscribe);
    return trackedUnsubscribe;
  }

  abort(pluginId: string): void {
    this.abortControllersByPlugin.get(pluginId)?.forEach((controller) => controller.abort());
    this.abortControllersByPlugin.delete(pluginId);
    this.reviewUnsubscribersByPlugin.get(pluginId)?.forEach((unsubscribe) => unsubscribe());
    this.reviewUnsubscribersByPlugin.delete(pluginId);
  }
}
