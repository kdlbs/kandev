import type { Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";

export type AssetEntry = {
  name: string;
  type: "screenshot" | "recording";
  format: "png" | "gif" | "webm";
  file: string;
  test: string;
  caption?: string;
};

export type AssetManifest = {
  generated_at: string;
  assets: AssetEntry[];
};

const DEFAULT_OUTPUT_DIR = path.resolve(__dirname, "../../.pr-assets");
const FRAMES_DIR = ".frames";
const RECORDING_FPS = 5;
const RECORDING_INTERVAL = 1000 / RECORDING_FPS;

// Ensures the output directory is wiped exactly once per process (per e2e run).
let _cleanedForRun = false;

function cleanOutputDir(outputDir: string): void {
  if (_cleanedForRun) return;
  _cleanedForRun = true;
  if (fs.existsSync(outputDir)) {
    fs.rmSync(outputDir, { recursive: true });
  }
  fs.mkdirSync(outputDir, { recursive: true });
}

function sanitizeName(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

function getTestSlug(testFile: string): string {
  const base = path.basename(testFile, path.extname(testFile));
  return sanitizeName(base.replace(/\.spec$/, ""));
}

function ensureDir(dir: string): void {
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

function readManifest(outputDir: string): AssetManifest {
  const manifestPath = path.join(outputDir, "manifest.json");
  if (fs.existsSync(manifestPath)) {
    return JSON.parse(fs.readFileSync(manifestPath, "utf-8")) as AssetManifest;
  }
  return { generated_at: new Date().toISOString(), assets: [] };
}

function writeManifest(outputDir: string, manifest: AssetManifest): void {
  manifest.generated_at = new Date().toISOString();
  fs.writeFileSync(path.join(outputDir, "manifest.json"), JSON.stringify(manifest, null, 2));
}

export class PrAssetCapture {
  private readonly page: Page;
  private readonly outputDir: string;
  private readonly enabled: boolean;
  private readonly testSlug: string;
  private readonly assets: AssetEntry[] = [];
  private recordingName: string | null = null;
  private recordingInterval: ReturnType<typeof setInterval> | null = null;
  private recordingFrameCount = 0;

  constructor(page: Page, testFile: string, opts?: { outputDir?: string }) {
    this.page = page;
    this.enabled = !!process.env.CAPTURE_PR_ASSETS;
    this.outputDir = opts?.outputDir ?? DEFAULT_OUTPUT_DIR;
    this.testSlug = getTestSlug(testFile);

    if (this.enabled) {
      cleanOutputDir(this.outputDir);
    }
  }

  async screenshot(name: string, opts?: { caption?: string; fullPage?: boolean }): Promise<void> {
    if (!this.enabled) return;

    const safeName = sanitizeName(name);
    const fileName = `${this.testSlug}--${safeName}.png`;
    const filePath = path.join(this.outputDir, fileName);

    await this.page.screenshot({
      path: filePath,
      fullPage: opts?.fullPage ?? false,
    });

    this.assets.push({
      name: safeName,
      type: "screenshot",
      format: "png",
      file: fileName,
      test: `${this.testSlug}.spec.ts`,
      caption: opts?.caption,
    });
  }

  async startRecording(name: string): Promise<void> {
    if (!this.enabled) return;
    if (this.recordingName) {
      await this.stopRecording();
    }

    const safeName = sanitizeName(name);
    this.recordingName = safeName;
    this.recordingFrameCount = 0;

    const framesDir = path.join(this.outputDir, FRAMES_DIR, safeName);
    ensureDir(framesDir);

    this.recordingInterval = setInterval(async () => {
      try {
        const frameNum = String(this.recordingFrameCount++).padStart(4, "0");
        await this.page.screenshot({
          path: path.join(framesDir, `frame-${frameNum}.png`),
        });
      } catch {
        // Page may have navigated — ignore frame capture errors
      }
    }, RECORDING_INTERVAL);
  }

  async stopRecording(opts?: { caption?: string }): Promise<void> {
    if (!this.enabled || !this.recordingName || !this.recordingInterval) return;

    clearInterval(this.recordingInterval);
    this.recordingInterval = null;

    const safeName = this.recordingName;
    this.recordingName = null;

    // Take one final frame
    const framesDir = path.join(this.outputDir, FRAMES_DIR, safeName);
    try {
      const frameNum = String(this.recordingFrameCount).padStart(4, "0");
      await this.page.screenshot({
        path: path.join(framesDir, `frame-${frameNum}.png`),
      });
    } catch {
      // ignore
    }

    // Register the asset — conversion happens in the post-processing script
    const fileName = `${this.testSlug}--${safeName}.gif`;
    this.assets.push({
      name: safeName,
      type: "recording",
      format: "gif",
      file: fileName,
      test: `${this.testSlug}.spec.ts`,
      caption: opts?.caption,
    });
  }

  /** Write captured assets to the manifest. Call once after test completes. */
  flush(): void {
    if (!this.enabled || this.assets.length === 0) return;

    // Stop any in-progress recording
    if (this.recordingInterval) {
      clearInterval(this.recordingInterval);
      this.recordingInterval = null;
    }

    const manifest = readManifest(this.outputDir);
    // Remove stale entries from this test
    manifest.assets = manifest.assets.filter((a) => a.test !== `${this.testSlug}.spec.ts`);
    manifest.assets.push(...this.assets);
    writeManifest(this.outputDir, manifest);
  }
}
