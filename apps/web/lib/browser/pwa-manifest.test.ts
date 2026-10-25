import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

type PwaManifest = {
  display?: string;
  display_override?: string[];
};

describe("PWA manifest window controls", () => {
  it("prefers the fused title bar and preserves standalone fallback", () => {
    const manifest = JSON.parse(
      readFileSync(resolve(process.cwd(), "public/manifest.webmanifest"), "utf8"),
    ) as PwaManifest;

    expect(manifest.display_override).toEqual(["window-controls-overlay", "standalone"]);
    expect(manifest.display).toBe("standalone");
  });
});
