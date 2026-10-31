import { readFile, readdir, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { gzip } from "node:zlib";

const compress = promisify(gzip);

/** @param {string} outDir Resolved Vite output directory. */
export async function precompressAssets(outDir) {
  for (const entry of await readdir(outDir, { withFileTypes: true })) {
    const filename = path.join(outDir, entry.name);
    if (entry.isDirectory()) {
      await precompressAssets(filename);
    } else if (entry.isFile() && /\.(js|css)$/.test(entry.name)) {
      await precompressFile(filename);
    }
  }
}

/** @param {string} filename */
async function precompressFile(filename) {
  const sibling = `${filename}.gz`;
  await rm(sibling, { force: true });
  const original = await readFile(filename);
  if (original.length < 1024) return;
  const compressed = await compress(original, { level: 9 });
  if (compressed.length >= original.length) return;
  const temporary = `${sibling}.tmp`;
  try {
    await writeFile(temporary, compressed);
    await rename(temporary, sibling);
  } finally {
    await rm(temporary, { force: true });
  }
}

/** @returns {import("vite").Plugin} */
export function precompressedAssets() {
  let outDir = "";
  return {
    name: "kandev:precompressed-assets",
    apply: "build",
    configResolved(config) {
      outDir = path.resolve(config.root, config.build.outDir);
    },
    async closeBundle() {
      await precompressAssets(outDir);
    },
  };
}
