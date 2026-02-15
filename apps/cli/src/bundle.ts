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
  // Next.js standalone output location depends on workspace structure:
  //   - non-monorepo: web/server.js
  //   - monorepo (apps/ root): web/web/server.js
  //   - monorepo (project root): web/apps/web/server.js
  const candidates = [
    path.join(bundleDir, "web", "server.js"),
    path.join(bundleDir, "web", "web", "server.js"),
    path.join(bundleDir, "web", "apps", "web", "server.js"),
  ];
  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }
  return null;
}
