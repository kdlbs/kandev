import fs from "node:fs";
import path from "node:path";

const ABOVE_DEFAULT_LIMIT_BYTES = 16 * 1024 * 1024 * 1024;

export function seedManagedGoCache(tmpDir: string): { artifact: string } {
  const cacheRoot = path.join(tmpDir, ".kandev", "cache");
  const cachePath = path.join(cacheRoot, "go-build");
  const artifact = path.join(cachePath, "e2e-sparse-artifact");
  fs.mkdirSync(cachePath, { recursive: true });
  fs.writeFileSync(path.join(cachePath, ".go-build.kandev-owned"), "kandev-managed-go-cache\n");
  fs.closeSync(fs.openSync(artifact, "w"));
  fs.truncateSync(artifact, ABOVE_DEFAULT_LIMIT_BYTES);
  return { artifact };
}
