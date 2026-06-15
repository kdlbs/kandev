import * as tar from "tar";
import fs from "node:fs";
import path from "node:path";

export function extractTarGz(archivePath: string, destDir: string): void {
  tar.extract({ file: archivePath, cwd: destDir, sync: true });
}

export function ensureExtracted(archivePath: string, destDir: string): void {
  const marker = path.join(destDir, ".extracted");
  if (fs.existsSync(marker)) {
    return;
  }
  extractTarGz(archivePath, destDir);
  fs.writeFileSync(marker, "");
}

export function findBundleRoot(cacheDir: string): string {
  const candidate = path.join(cacheDir, "kandev");
  if (fs.existsSync(candidate)) {
    return candidate;
  }
  return cacheDir;
}
