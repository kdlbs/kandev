import { toast as upstreamToast } from "sonner";

import { scheduleFrontendErrorReport } from "@/lib/api/domains/frontend-error-log-api";

const reportingError = (
  ...args: Parameters<typeof upstreamToast.error>
): ReturnType<typeof upstreamToast.error> => {
  const result = upstreamToast.error(...args);
  const options = args[1];
  scheduleFrontendErrorReport({
    source: "sonner",
    title: args[0],
    description: ownDataValue(options, "description"),
    error: args[0] instanceof Error ? args[0] : ownDataValue(options, "error"),
  });
  return result;
};

export const toast = new Proxy(upstreamToast, {
  get(target, property, receiver) {
    if (property === "error") return reportingError;
    return Reflect.get(target, property, receiver);
  },
}) as typeof upstreamToast;

/**
 * `host.toast` for one plugin (see `lib/plugins/host-api.ts`). Identical to
 * `toast` except `.error` logs to the console instead of filing a frontend
 * error report.
 *
 * That endpoint is kandev's *own* diagnostics: the backend logs each report at
 * Error level as "frontend error toast". A third-party plugin's error toast is
 * not a kandev application error — a plugin that toasts a failed poll every 60s
 * would file an application error every 60s indefinitely, indistinguishable
 * from a real one. (The backend also accepts only a closed set of sources, so
 * a per-plugin `source` is rejected with 400 rather than attributed.)
 *
 * Plugin failures go to the console under the `[plugins]` prefix instead,
 * matching every other plugin extension point — `PluginErrorBoundary`,
 * `dispatchToPluginWsHandlers`, and the `onThemeChange` listener guard.
 */
export function createPluginToast(pluginId: string): typeof upstreamToast {
  const pluginError = (
    ...args: Parameters<typeof upstreamToast.error>
  ): ReturnType<typeof upstreamToast.error> => {
    const result = upstreamToast.error(...args);
    console.error(`[plugins] toast.error from "${pluginId}":`, args[0]);
    return result;
  };
  return new Proxy(upstreamToast, {
    get(target, property, receiver) {
      if (property === "error") return pluginError;
      return Reflect.get(target, property, receiver);
    },
  }) as typeof upstreamToast;
}

function ownDataValue(value: unknown, property: string): unknown {
  if (!value || typeof value !== "object") return undefined;
  try {
    return Object.getOwnPropertyDescriptor(value, property)?.value;
  } catch {
    return undefined;
  }
}
