#!/usr/bin/env tsx
/**
 * Post-e2e script: converts frame sequences to GIFs using ffmpeg.
 * Falls back gracefully if ffmpeg is not available.
 *
 * Usage: npx tsx e2e/scripts/convert-assets.ts
 */

import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import type { AssetManifest } from "../helpers/pr-asset-capture";

const ASSETS_DIR = path.resolve(__dirname, "../../.pr-assets");
const FRAMES_DIR = path.join(ASSETS_DIR, ".frames");
const MANIFEST_PATH = path.join(ASSETS_DIR, "manifest.json");
const GIF_WIDTH = 800;
const GIF_FPS = 5;

function hasFfmpeg(): boolean {
  try {
    execSync("ffmpeg -version", { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

function convertFramesToGif(framesDir: string, outputPath: string): boolean {
  try {
    const cmd = [
      "ffmpeg",
      "-y",
      `-framerate ${GIF_FPS}`,
      `-i "${path.join(framesDir, "frame-%04d.png")}"`,
      `-vf "fps=${GIF_FPS},scale=${GIF_WIDTH}:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse"`,
      "-loop 0",
      `"${outputPath}"`,
    ].join(" ");

    execSync(cmd, { stdio: "ignore" });
    return true;
  } catch (err) {
    console.error(`Failed to convert frames to GIF: ${outputPath}`, err);
    return false;
  }
}

function cleanupFrames(recordingName: string): void {
  const framesPath = path.join(FRAMES_DIR, recordingName);
  if (fs.existsSync(framesPath)) {
    fs.rmSync(framesPath, { recursive: true });
  }
}

function main(): void {
  if (!fs.existsSync(MANIFEST_PATH)) {
    console.log("No manifest.json found — nothing to convert.");
    return;
  }

  const manifest: AssetManifest = JSON.parse(fs.readFileSync(MANIFEST_PATH, "utf-8"));
  const recordings = manifest.assets.filter((a) => a.type === "recording");

  if (recordings.length === 0) {
    console.log("No recordings to convert.");
    return;
  }

  const ffmpegAvailable = hasFfmpeg();
  if (!ffmpegAvailable) {
    console.warn("ffmpeg not found — skipping GIF conversion. Install ffmpeg for GIF support.");
    // Keep recordings in manifest but mark as unconverted
    for (const rec of recordings) {
      const framesDir = path.join(FRAMES_DIR, rec.name);
      if (!fs.existsSync(framesDir)) continue;
      // Fall back: use first frame as a static screenshot
      const firstFrame = path.join(framesDir, "frame-0000.png");
      if (fs.existsSync(firstFrame)) {
        const fallbackName = rec.file.replace(".gif", ".png");
        fs.copyFileSync(firstFrame, path.join(ASSETS_DIR, fallbackName));
        rec.file = fallbackName;
        rec.format = "png";
        rec.type = "screenshot";
        console.log(`  Fallback: ${rec.name} → ${fallbackName} (first frame)`);
      }
      cleanupFrames(rec.name);
    }
    manifest.generated_at = new Date().toISOString();
    fs.writeFileSync(MANIFEST_PATH, JSON.stringify(manifest, null, 2));
    return;
  }

  let converted = 0;
  for (const rec of recordings) {
    const framesDir = path.join(FRAMES_DIR, rec.name);
    if (!fs.existsSync(framesDir)) {
      console.warn(`  Frames directory not found for: ${rec.name}`);
      continue;
    }

    const outputPath = path.join(ASSETS_DIR, rec.file);
    console.log(`  Converting: ${rec.name} → ${rec.file}`);
    if (convertFramesToGif(framesDir, outputPath)) {
      converted++;
      cleanupFrames(rec.name);
    }
  }

  // Clean up .frames dir if empty
  if (fs.existsSync(FRAMES_DIR)) {
    const remaining = fs.readdirSync(FRAMES_DIR);
    if (remaining.length === 0) {
      fs.rmSync(FRAMES_DIR, { recursive: true });
    }
  }

  manifest.generated_at = new Date().toISOString();
  fs.writeFileSync(MANIFEST_PATH, JSON.stringify(manifest, null, 2));
  console.log(`Converted ${converted}/${recordings.length} recordings to GIF.`);
}

main();
