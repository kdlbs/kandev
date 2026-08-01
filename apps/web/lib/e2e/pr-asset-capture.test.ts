import type { Page } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import { PrAssetCapture } from "../../e2e/helpers/pr-asset-capture";

const originalCaptureSetting = process.env.CAPTURE_PR_ASSETS;
const MANIFEST_FILE = "manifest.json";

afterEach(() => {
  if (originalCaptureSetting === undefined) {
    delete process.env.CAPTURE_PR_ASSETS;
    return;
  }
  process.env.CAPTURE_PR_ASSETS = originalCaptureSetting;
});

describe("PrAssetCapture", () => {
  it("replaces stale entries without removing assets from another worker", async () => {
    process.env.CAPTURE_PR_ASSETS = "1";
    const testFile = "mobile-capture.spec.ts";
    const outputDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-pr-assets-"));
    const page = {
      screenshot: async ({ path: screenshotPath }: { path: string }) => {
        fs.writeFileSync(screenshotPath, "screenshot");
      },
    } as unknown as Page;
    try {
      const existingAsset = path.join(outputDir, "desktop.png");
      fs.writeFileSync(existingAsset, "existing asset");
      fs.writeFileSync(
        path.join(outputDir, MANIFEST_FILE),
        JSON.stringify({
          generated_at: "2026-01-01T00:00:00.000Z",
          assets: [
            {
              name: "stale",
              type: "screenshot",
              format: "png",
              file: "mobile-capture--stale.png",
              test: testFile,
            },
            {
              name: "desktop",
              type: "screenshot",
              format: "png",
              file: "desktop.png",
              test: "desktop-capture.spec.ts",
            },
          ],
        }),
      );

      const capture = new PrAssetCapture(page, `/tmp/${testFile}`, { outputDir });
      await capture.screenshot("fresh");

      capture.flush();

      expect(fs.existsSync(existingAsset)).toBe(true);
      const manifest = JSON.parse(fs.readFileSync(path.join(outputDir, MANIFEST_FILE), "utf-8"));
      expect(manifest.assets.map((asset: { file: string }) => asset.file).sort()).toEqual([
        "desktop.png",
        "mobile-capture--fresh.png",
      ]);
    } finally {
      fs.rmSync(outputDir, { recursive: true, force: true });
    }
  });

  it("reclaims a stale manifest lock before flushing", async () => {
    process.env.CAPTURE_PR_ASSETS = "1";
    const outputDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-pr-assets-"));
    const page = {
      screenshot: async ({ path: screenshotPath }: { path: string }) => {
        fs.writeFileSync(screenshotPath, "screenshot");
      },
    } as unknown as Page;
    try {
      const lockDir = path.join(outputDir, ".manifest.lock");
      fs.mkdirSync(lockDir);
      const staleAt = new Date(Date.now() - 31_000);
      fs.utimesSync(lockDir, staleAt, staleAt);

      const capture = new PrAssetCapture(page, "/tmp/stale-lock.spec.ts", { outputDir });
      await capture.screenshot("recovered");
      capture.flush();

      expect(fs.existsSync(lockDir)).toBe(false);
      const manifest = JSON.parse(fs.readFileSync(path.join(outputDir, MANIFEST_FILE), "utf-8"));
      expect(manifest.assets.map((asset: { file: string }) => asset.file)).toEqual([
        "stale-lock--recovered.png",
      ]);
    } finally {
      fs.rmSync(outputDir, { recursive: true, force: true });
    }
  });

  it("retains assets flushed by separate captures", async () => {
    process.env.CAPTURE_PR_ASSETS = "1";
    const outputDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-pr-assets-"));
    const page = {
      screenshot: async ({ path: screenshotPath }: { path: string }) => {
        fs.writeFileSync(screenshotPath, "screenshot");
      },
    } as unknown as Page;
    try {
      const desktop = new PrAssetCapture(page, "/tmp/desktop-capture.spec.ts", { outputDir });
      const mobile = new PrAssetCapture(page, "/tmp/mobile-capture.spec.ts", { outputDir });
      await desktop.screenshot("desktop");
      await mobile.screenshot("mobile");

      desktop.flush();
      mobile.flush();

      const manifest = JSON.parse(fs.readFileSync(path.join(outputDir, MANIFEST_FILE), "utf-8"));
      expect(manifest.assets.map((asset: { file: string }) => asset.file).sort()).toEqual([
        "desktop-capture--desktop.png",
        "mobile-capture--mobile.png",
      ]);
    } finally {
      fs.rmSync(outputDir, { recursive: true, force: true });
    }
  });
});
