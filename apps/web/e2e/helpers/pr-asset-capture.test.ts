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

  // A cross-device spec drives two clients from one capture instance: a second
  // instance cannot work, because flush() clears the spec's existing manifest
  // entries and the last flush would drop the first screenshot.
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
});
