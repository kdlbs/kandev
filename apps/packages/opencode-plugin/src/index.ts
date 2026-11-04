import type * as OpenCode from "@opencode/plugin";

import { startCore, type OpenCodePluginContext } from "./core/core.js";
import { productionBehaviors } from "./behaviors/registry.js";

/**
 * The single Kandev-supplied OpenCode v2 server plugin. The loader resolves the
 * default export and calls setup once per server location. Cleanup aborts the
 * bridge and every behavior.
 */
const plugin: OpenCode.Plugin.Plugin = {
  id: "kandev",
  async setup(context) {
    try {
      return await startCore(context as unknown as OpenCodePluginContext, productionBehaviors());
    } catch {
      // A failed integration must never make the ACP conversation unusable.
      console.warn("[kandev-opencode-plugin] integration_unavailable");
      return async () => undefined;
    }
  },
};

export default plugin;
