import type { DesktopEventTransport } from "./adapter";
import type { DesktopInvokeTransport } from "./updater-adapter";

type TauriEvent = { payload: unknown };

export type TauriEventInternals = {
  invoke: (command: string, args?: Record<string, unknown>) => Promise<unknown>;
  transformCallback: (callback: (event: TauriEvent) => void, once?: boolean) => number;
};

type TauriWindow = Window & {
  __TAURI_INTERNALS__?: TauriEventInternals;
};

type InternalsProvider = () => TauriEventInternals | undefined;

function browserInternals(): TauriEventInternals | undefined {
  if (typeof window === "undefined") return undefined;
  return (window as TauriWindow).__TAURI_INTERNALS__;
}

export function createTauriEventTransport(
  getInternals: InternalsProvider = browserInternals,
): DesktopEventTransport {
  return {
    isAvailable: () => Boolean(getInternals()),
    listen: async (eventName, listener) => {
      const internals = getInternals();
      if (!internals) return () => undefined;
      const handler = internals.transformCallback((event) => listener(event.payload));
      const eventId = (await internals.invoke("plugin:event|listen", {
        event: eventName,
        handler,
        target: { kind: "Any" },
      })) as number;
      return () => {
        void internals.invoke("plugin:event|unlisten", { event: eventName, eventId });
      };
    },
  };
}

export function createTauriInvokeTransport(
  getInternals: InternalsProvider = browserInternals,
): DesktopInvokeTransport {
  return {
    isAvailable: () => Boolean(getInternals()),
    invoke: async (command, args) => {
      const internals = getInternals();
      if (!internals) throw new Error("The desktop updater is unavailable.");
      return internals.invoke(command, args);
    },
  };
}
