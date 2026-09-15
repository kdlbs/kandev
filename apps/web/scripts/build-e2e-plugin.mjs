import { createHash } from "node:crypto";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const argumentIndex = process.argv.indexOf("--outDir");
if (argumentIndex === -1 || !process.argv[argumentIndex + 1]) {
  throw new Error("build:e2e-plugin requires --outDir");
}

const source = path.resolve("e2e/fixtures/plugins/prompt-history-plugin/bundle.js");
const outputDirectory = path.resolve(process.argv[argumentIndex + 1]);
const output = path.join(outputDirectory, "bundle.js");
const content = await readFile(source);
const sourceHash = createHash("sha256").update(content).digest("hex");

await rm(outputDirectory, { recursive: true, force: true });
await mkdir(outputDirectory, { recursive: true });
await writeFile(output, content);

const generated = await readFile(output);
const generatedHash = createHash("sha256").update(generated).digest("hex");
if (generatedHash !== sourceHash) {
  throw new Error("build:e2e-plugin generated stale output");
}
console.log(`[build:e2e-plugin] ${sourceHash} -> ${output}`);
