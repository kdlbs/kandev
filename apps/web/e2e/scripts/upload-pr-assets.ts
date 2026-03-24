#!/usr/bin/env tsx
/**
 * Generate a markdown snippet from captured PR assets.
 *
 * Reads .pr-assets/manifest.json and produces .pr-assets/embed.md with
 * placeholder markdown for each asset. The user (or PR skill) then drags
 * and drops the actual image files into the PR description on GitHub.
 *
 * Usage: npx tsx e2e/scripts/upload-pr-assets.ts [<pr-number>]
 *
 * Outputs: .pr-assets/embed.md
 */

import fs from "node:fs";
import path from "node:path";
import type { AssetManifest, AssetEntry } from "../helpers/pr-asset-capture";

const ASSETS_DIR = path.resolve(__dirname, "../../.pr-assets");
const MANIFEST_PATH = path.join(ASSETS_DIR, "manifest.json");
const EMBED_PATH = path.join(ASSETS_DIR, "embed.md");

function generateMarkdown(assets: AssetEntry[]): string {
  if (assets.length === 0) return "";

  const lines: string[] = ["## Screenshots", ""];

  for (const asset of assets) {
    const caption = asset.caption ?? asset.name.replace(/-/g, " ");
    if (asset.format === "gif") {
      lines.push(`### ${caption}`);
      lines.push("");
      lines.push(`<!-- Drag and drop .pr-assets/${asset.file} here -->`);
      lines.push("");
    } else {
      lines.push(`**${caption}**`);
      lines.push("");
      lines.push(`<!-- Drag and drop .pr-assets/${asset.file} here -->`);
      lines.push("");
    }
  }

  return lines.join("\n");
}

function main(): void {
  if (!fs.existsSync(MANIFEST_PATH)) {
    console.log("No manifest.json found — nothing to process.");
    process.exit(0);
  }

  const manifest: AssetManifest = JSON.parse(fs.readFileSync(MANIFEST_PATH, "utf-8"));
  const available = manifest.assets.filter((a) => fs.existsSync(path.join(ASSETS_DIR, a.file)));

  if (available.length === 0) {
    console.log("No asset files found on disk.");
    process.exit(0);
  }

  const markdown = generateMarkdown(available);
  fs.writeFileSync(EMBED_PATH, markdown);

  console.log(`Generated: ${EMBED_PATH}`);
  console.log(`\nAssets ready (${available.length} files):`);
  for (const asset of available) {
    const filePath = path.join(ASSETS_DIR, asset.file);
    const size = (fs.statSync(filePath).size / 1024).toFixed(1);
    console.log(`  .pr-assets/${asset.file} (${size} KB)`);
  }
  console.log(
    "\nTo add screenshots to the PR, drag and drop the files from .pr-assets/ into the PR description on GitHub.",
  );
}

main();
