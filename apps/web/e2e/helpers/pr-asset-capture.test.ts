import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Page } from "@playwright/test";
import { PrAssetCapture } from "./pr-asset-capture";

function fakePage(): Page {
  return { screenshot: vi.fn().mockResolvedValue(undefined) } as unknown as Page;
}

let outputDir: string;

beforeEach(() => {
  process.env.CAPTURE_PR_ASSETS = "1";
  outputDir = fs.mkdtempSync(path.join(os.tmpdir(), "pr-assets-"));
});

afterEach(() => {
  delete process.env.CAPTURE_PR_ASSETS;
  fs.rmSync(outputDir, { recursive: true, force: true });
});

describe("PrAssetCapture.screenshot", () => {
  it("shoots the constructor page by default", async () => {
    const primary = fakePage();
    const capture = new PrAssetCapture(primary, "cross-device.spec.ts", { outputDir });

    await capture.screenshot("desktop");

    expect(primary.screenshot).toHaveBeenCalledTimes(1);
  });

  // A cross-device test can drive two clients from one capture instance. Tests
  // with separate capture lifecycles use distinct captureKey values instead.
  it("shoots the page override so one instance can capture several clients", async () => {
    const primary = fakePage();
    const secondary = fakePage();
    const capture = new PrAssetCapture(primary, "cross-device.spec.ts", { outputDir });

    await capture.screenshot("desktop");
    await capture.screenshot("mobile", { page: secondary });
    capture.flush();

    expect(primary.screenshot).toHaveBeenCalledTimes(1);
    expect(secondary.screenshot).toHaveBeenCalledTimes(1);
    const manifest = JSON.parse(fs.readFileSync(path.join(outputDir, "manifest.json"), "utf-8"));
    expect(manifest.assets.map((asset: { name: string }) => asset.name)).toEqual([
      "desktop",
      "mobile",
    ]);
  });

  it("captures nothing when CAPTURE_PR_ASSETS is unset", async () => {
    delete process.env.CAPTURE_PR_ASSETS;
    const primary = fakePage();
    const secondary = fakePage();
    const capture = new PrAssetCapture(primary, "cross-device.spec.ts", { outputDir });

    await capture.screenshot("desktop", { page: secondary });

    expect(primary.screenshot).not.toHaveBeenCalled();
    expect(secondary.screenshot).not.toHaveBeenCalled();
  });

  it("keeps assets from separate capture keys in one spec file", async () => {
    const page = fakePage();
    const desktop = new PrAssetCapture(page, "packaged-plugin.spec.ts", {
      outputDir,
      captureKey: "desktop",
    });
    const mobile = new PrAssetCapture(page, "packaged-plugin.spec.ts", {
      outputDir,
      captureKey: "mobile",
    });

    await desktop.screenshot("workbench");
    desktop.flush();
    await mobile.screenshot("filters");
    mobile.flush();

    const manifest = JSON.parse(fs.readFileSync(path.join(outputDir, "manifest.json"), "utf-8"));
    expect(manifest.assets.map((asset: { name: string }) => asset.name)).toEqual([
      "workbench",
      "filters",
    ]);
  });
});
