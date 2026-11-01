import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import { defaultFeatureFlags } from "./types";

const backendConfigPath = resolve(process.cwd(), "../backend/internal/common/config/config.go");

function backendFeatureKeys(source: string): string[] {
  const structBody = source.match(/type FeaturesConfig struct \{([\s\S]*?)\n\}/)?.[1];
  if (!structBody) {
    throw new Error("FeaturesConfig declaration not found in backend config");
  }

  return Array.from(structBody.matchAll(/json:"([^",]+)(?:,[^"]*)?"/g), (match) => match[1]).sort();
}

describe("feature flag repository contract", () => {
  it("omits the graduated interrupted-conversation continuation flag", () => {
    expect(defaultFeatureFlags).not.toHaveProperty("providerInterruptionContinuation");
  });
  it("omits retired Office session identity from frontend defaults", () => {
    expect(defaultFeatureFlags).not.toHaveProperty("officeSessionIdentity");
  });

  it("keeps browser-owned LSP continuity disabled by default", () => {
    expect(defaultFeatureFlags.lspBrowserContinuity).toBe(false);
  });

  it("keeps dynamic agent routing disabled by default", () => {
    expect(defaultFeatureFlags.dynamicAgentRouting).toBe(false);
  });

  it("keeps canvases disabled by default", () => {
    expect(defaultFeatureFlags.canvases).toBe(false);
  });

  it("keeps Integrated control opt-in in the frontend", () => {
    expect(defaultFeatureFlags.integratedControl).toBe(false);
  });

  it("keeps the background-work browser fallback false until hydration", () => {
    // Shipped profiles enable the feature, but the browser must not invent a
    // default of its own before the authoritative value arrives.
    expect(defaultFeatureFlags.agentBackgroundWork).toBe(false);
  });

  it("omits the graduated remote executor plugins flag", () => {
    expect(defaultFeatureFlags).not.toHaveProperty("remoteExecutorPlugins");
  });

  it("keeps frontend defaults equal to backend FeaturesConfig JSON keys", async () => {
    const backendConfig = await readFile(backendConfigPath, "utf8");

    expect(Object.keys(defaultFeatureFlags).sort()).toEqual(backendFeatureKeys(backendConfig));
  });
});
