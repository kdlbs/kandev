import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Page } from "@playwright/test";
import { PrAssetCapture, type AssetManifest } from "./pr-asset-capture";

function fakePage(content = "screenshot"): Page {
  return {
    screenshot: vi.fn(async ({ path: screenshotPath }: { path: string }) => {
      fs.writeFileSync(screenshotPath, content);
    }),
  } as unknown as Page;
}

let outputDir: string;

function readManifest(): AssetManifest {
  return JSON.parse(fs.readFileSync(path.join(outputDir, "manifest.json"), "utf-8"));
}

beforeEach(() => {
  vi.stubEnv("CAPTURE_PR_ASSETS", "1");
  outputDir = fs.mkdtempSync(path.join(os.tmpdir(), "pr-assets-"));
});

afterEach(() => {
  vi.unstubAllEnvs();
  fs.rmSync(outputDir, { recursive: true, force: true });
});

describe("PrAssetCapture.screenshot", () => {
  it("shoots the constructor page by default", async () => {
    const primary = fakePage("desktop pixels");
    const capture = new PrAssetCapture(primary, "cross-device.spec.ts", { outputDir });

    await capture.screenshot("desktop");
    capture.flush();

    const file = path.join(outputDir, "cross-device--desktop.png");
    expect(primary.screenshot).toHaveBeenCalledExactlyOnceWith({ path: file, fullPage: false });
    expect(fs.readFileSync(file, "utf8")).toBe("desktop pixels");
    expect(readManifest().assets).toEqual([
      {
        name: "desktop",
        type: "screenshot",
        format: "png",
        file: "cross-device--desktop.png",
        test: "cross-device.spec.ts",
      },
    ]);
  });

  it("shoots the page override so one instance can capture several clients", async () => {
    const primary = fakePage("desktop pixels");
    const secondary = fakePage("mobile pixels");
    const capture = new PrAssetCapture(primary, "cross-device.spec.ts", { outputDir });

    await capture.screenshot("desktop");
    await capture.screenshot("mobile", { page: secondary, fullPage: true });
    capture.flush();

    expect(primary.screenshot).toHaveBeenCalledTimes(1);
    const mobileFile = path.join(outputDir, "cross-device--mobile.png");
    expect(secondary.screenshot).toHaveBeenCalledExactlyOnceWith({
      path: mobileFile,
      fullPage: true,
    });
    expect(fs.readFileSync(path.join(outputDir, "cross-device--desktop.png"), "utf8")).toBe(
      "desktop pixels",
    );
    expect(fs.readFileSync(mobileFile, "utf8")).toBe("mobile pixels");
    expect(readManifest().assets.map((asset) => asset.file)).toEqual([
      "cross-device--desktop.png",
      "cross-device--mobile.png",
    ]);
    expect(readManifest().assets.map((asset) => asset.name)).toEqual(["desktop", "mobile"]);
  });

  it("captures nothing when CAPTURE_PR_ASSETS is unset", async () => {
    vi.stubEnv("CAPTURE_PR_ASSETS", undefined);
    const primary = fakePage();
    const secondary = fakePage();
    const capture = new PrAssetCapture(primary, "cross-device.spec.ts", { outputDir });

    await capture.screenshot("desktop", { page: secondary });
    capture.flush();

    expect(primary.screenshot).not.toHaveBeenCalled();
    expect(secondary.screenshot).not.toHaveBeenCalled();
    expect(fs.readdirSync(outputDir)).toEqual([]);
  });
});

describe("PrAssetCapture.flush", () => {
  it.each([
    {
      scenario: "separate spec files",
      desktopFile: "desktop-capture.spec.ts",
      mobileFile: "mobile-capture.spec.ts",
      desktopKey: undefined,
      mobileKey: undefined,
      expectedFiles: ["desktop-capture--workbench.png", "mobile-capture--filters.png"],
    },
    {
      scenario: "separate capture keys in one spec file",
      desktopFile: "packaged-plugin.spec.ts",
      mobileFile: "packaged-plugin.spec.ts",
      desktopKey: "desktop",
      mobileKey: "mobile",
      expectedFiles: [
        "packaged-plugin-desktop--workbench.png",
        "packaged-plugin-mobile--filters.png",
      ],
    },
  ])("retains assets from $scenario", async (testCase) => {
    const desktop = new PrAssetCapture(fakePage("desktop pixels"), testCase.desktopFile, {
      outputDir,
      captureKey: testCase.desktopKey,
    });
    const mobile = new PrAssetCapture(fakePage("mobile pixels"), testCase.mobileFile, {
      outputDir,
      captureKey: testCase.mobileKey,
    });

    await desktop.screenshot("workbench");
    desktop.flush();
    await mobile.screenshot("filters");
    mobile.flush();

    expect(readManifest().assets.map((asset) => asset.file)).toEqual(testCase.expectedFiles);
    expect(readManifest().assets.map((asset) => asset.name)).toEqual(["workbench", "filters"]);
    expect(fs.readFileSync(path.join(outputDir, testCase.expectedFiles[0]), "utf8")).toBe(
      "desktop pixels",
    );
    expect(fs.readFileSync(path.join(outputDir, testCase.expectedFiles[1]), "utf8")).toBe(
      "mobile pixels",
    );
  });

  it("replaces stale entries without removing assets from another worker", async () => {
    const existingAsset = path.join(outputDir, "desktop.png");
    fs.writeFileSync(existingAsset, "existing asset");
    fs.writeFileSync(
      path.join(outputDir, "manifest.json"),
      JSON.stringify({
        generated_at: "2026-01-01T00:00:00.000Z",
        assets: [
          {
            name: "stale",
            type: "screenshot",
            format: "png",
            file: "mobile-capture--stale.png",
            test: "mobile-capture.spec.ts",
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
    const capture = new PrAssetCapture(fakePage(), "mobile-capture.spec.ts", { outputDir });

    await capture.screenshot("fresh");
    capture.flush();

    expect(fs.readFileSync(existingAsset, "utf8")).toBe("existing asset");
    expect(
      readManifest()
        .assets.map((asset) => asset.file)
        .sort(),
    ).toEqual(["desktop.png", "mobile-capture--fresh.png"]);
  });

  it("reclaims a stale manifest lock before flushing", async () => {
    const lockDir = path.join(outputDir, ".manifest.lock");
    fs.mkdirSync(lockDir);
    const staleAt = new Date(Date.now() - 31_000);
    fs.utimesSync(lockDir, staleAt, staleAt);
    const capture = new PrAssetCapture(fakePage(), "stale-lock.spec.ts", { outputDir });

    await capture.screenshot("recovered");
    capture.flush();

    expect(fs.existsSync(lockDir)).toBe(false);
    expect(readManifest().assets.map((asset) => asset.file)).toEqual(["stale-lock--recovered.png"]);
  });
});
