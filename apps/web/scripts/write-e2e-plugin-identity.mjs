import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const [sourcePath, generatedPath, manifestPath, archivePath, identityPath] = process.argv.slice(2);
if (!identityPath)
  throw new Error(
    "write-e2e-plugin-identity requires source, output, manifest, archive, and identity paths",
  );

async function sha256(file) {
  return createHash("sha256")
    .update(await readFile(file))
    .digest("hex");
}

const manifest = await readFile(manifestPath, "utf8");
for (const required of [
  'id: "kandev-plugin-e2e"',
  'version: "1.0.0"',
  'min_kandev_version: "0.91.1"',
  'api_read: ["messages"]',
]) {
  if (!manifest.includes(required)) throw new Error(`fixture manifest missing ${required}`);
}
const output = await readFile(generatedPath, "utf8");
if (!output.includes('id: "prompt-history-plugin"')) {
  throw new Error("fixture output missing prompt-history-plugin panel");
}

const identity = {
  schemaVersion: 2,
  pluginId: "kandev-plugin-e2e",
  pluginVersion: "1.0.0",
  bundlePath: "ui/bundle.js",
  sourceHash: await sha256(sourcePath),
  generatedOutputHash: await sha256(generatedPath),
  manifestHash: await sha256(manifestPath),
  archiveSha256: await sha256(archivePath),
  manifestCapability: "messages",
  minKandevVersion: "0.91.1",
  panelKey: "prompt-history-plugin",
};
await writeFile(identityPath, `${JSON.stringify(identity, null, 2)}\n`);
console.log(`[e2e-plugin-identity] ${path.resolve(identityPath)}`);
