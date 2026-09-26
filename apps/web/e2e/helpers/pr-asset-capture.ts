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
const MANIFEST_LOCK_DIR = ".manifest.lock";
const MANIFEST_LOCK_RETRY_MS = 10;
const MANIFEST_LOCK_TIMEOUT_MS = 5_000;
const MANIFEST_LOCK_STALE_MS = 30_000;
const RECORDING_FPS = 5;
const RECORDING_INTERVAL = 1000 / RECORDING_FPS;

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

function waitForManifestLock(): void {
  const signal = new Int32Array(new SharedArrayBuffer(4));
  Atomics.wait(signal, 0, 0, MANIFEST_LOCK_RETRY_MS);
}

function reclaimStaleManifestLock(lockDir: string): boolean {
  try {
    const ageMs = Date.now() - fs.statSync(lockDir).mtimeMs;
    if (ageMs < MANIFEST_LOCK_STALE_MS) return false;
    fs.rmdirSync(lockDir);
    return true;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return true;
    throw error;
  }
}

function withManifestLock<T>(outputDir: string, write: () => T): T {
  const lockDir = path.join(outputDir, MANIFEST_LOCK_DIR);
  const deadline = Date.now() + MANIFEST_LOCK_TIMEOUT_MS;
  while (true) {
    try {
      fs.mkdirSync(lockDir);
      break;
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "EEXIST") throw error;
      if (reclaimStaleManifestLock(lockDir)) continue;
      if (Date.now() >= deadline) {
        throw new Error(`timed out waiting for PR asset manifest lock: ${lockDir}`);
      }
      waitForManifestLock();
    }
  }

  try {
    return write();
  } finally {
    fs.rmdirSync(lockDir);
  }
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

  constructor(page: Page, testFile: string, opts?: { outputDir?: string; captureKey?: string }) {
    this.page = page;
    this.enabled = !!process.env.CAPTURE_PR_ASSETS;
    this.outputDir = opts?.outputDir ?? DEFAULT_OUTPUT_DIR;
    const fileSlug = getTestSlug(testFile);
    const captureKey = sanitizeName(opts?.captureKey ?? "");
    this.testSlug = captureKey ? `${fileSlug}-${captureKey}` : fileSlug;

    if (this.enabled) {
      ensureDir(this.outputDir);
    }
  }

  /**
   * Whether captures are actually being written (`CAPTURE_PR_ASSETS`).
   *
   * Every capture method is a no-op otherwise, so a spec that does extra work
   * *for* a capture -- settling the UI, hiding transient chrome -- should skip
   * that work in ordinary runs rather than pay for it on every shard.
   */
  get capturing(): boolean {
    return this.enabled;
  }

  /**
   * Captures one asset. `opts.page` overrides the constructor page so a single
   * capture instance can shoot several clients in one test (a cross-device spec
   * drives two). Using a second instance instead would not work: flush() clears
   * this spec's existing manifest entries, so the last flush would win.
   */
  async screenshot(
    name: string,
    opts?: { caption?: string; fullPage?: boolean; page?: Page },
  ): Promise<void> {
    if (!this.enabled) return;

    const safeName = sanitizeName(name);
    const fileName = `${this.testSlug}--${safeName}.png`;
    const filePath = path.join(this.outputDir, fileName);

    await (opts?.page ?? this.page).screenshot({
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

    withManifestLock(this.outputDir, () => {
      const manifest = readManifest(this.outputDir);
      // Remove stale entries from this test.
      manifest.assets = manifest.assets.filter((a) => a.test !== `${this.testSlug}.spec.ts`);
      manifest.assets.push(...this.assets);
      writeManifest(this.outputDir, manifest);
    });
  }
}
