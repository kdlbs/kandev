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
  it("keeps dynamic agent routing disabled by default", () => {
    expect(defaultFeatureFlags.dynamicAgentRouting).toBe(false);
  });

  // The boot payload decides the real value; the frontend default has to be
  // false so a flag never reads as enabled before hydration.
  it("keeps agent stack reaping disabled until the boot payload says otherwise", () => {
    expect(defaultFeatureFlags.agentStackReaping).toBe(false);
  });

  it("keeps frontend defaults equal to backend FeaturesConfig JSON keys", async () => {
    const backendConfig = await readFile(backendConfigPath, "utf8");

    expect(Object.keys(defaultFeatureFlags).sort()).toEqual(backendFeatureKeys(backendConfig));
  });
});
