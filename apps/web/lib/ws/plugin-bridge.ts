/**
 * Bridges decoded WS notifications into plugin-registered handlers
 * (`registry.registerWsHandler(action, handler)`, PLUGIN-API.md). Called from
 * `lib/ws/client.ts` after the built-in `handlers` dispatch for a message, so
 * plugin handlers see the same notification vocabulary as the rest of the app.
 */
import { pluginRegistry } from "@/lib/plugins/registry";

/** Forwards `payload` to every plugin handler registered for `action`. Never throws. */
export function dispatchToPluginWsHandlers(action: string, payload: unknown): void {
  const handlers = pluginRegistry.getWsHandlers(action);
  for (const handler of handlers) {
    try {
      handler(payload);
    } catch (error) {
      console.error(`[plugins] ws handler for "${action}" threw`, error);
    }
  }
}
