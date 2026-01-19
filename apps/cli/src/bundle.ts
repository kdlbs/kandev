import AdmZip from "adm-zip";
import fs from "node:fs";
import path from "node:path";

export function extractZip(zipPath: string, destDir: string): void {
  const zip = new AdmZip(zipPath);
  zip.extractAllTo(destDir, true);
}

export function ensureExtracted(zipPath: string, destDir: string): void {
  const marker = path.join(destDir, ".extracted");
  if (fs.existsSync(marker)) {
    return;
  }
  extractZip(zipPath, destDir);
  fs.writeFileSync(marker, "");
}

export function findBundleRoot(cacheDir: string): string {
  const candidate = path.join(cacheDir, "kandev");
  if (fs.existsSync(candidate)) {
    return candidate;
  }
  return cacheDir;
}

export function resolveWebServerPath(bundleDir: string): string | null {
  const direct = path.join(bundleDir, "web", "server.js");
  if (fs.existsSync(direct)) {
    return direct;
  }
  const nested = path.join(bundleDir, "web", "apps", "web", "server.js");
  if (fs.existsSync(nested)) {
    return nested;
  }
  return null;
}
