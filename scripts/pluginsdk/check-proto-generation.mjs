import { readdir, readFile } from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { fileURLToPath, pathToFileURL } from "node:url";
import { resolve, join } from "node:path";

const run = promisify(execFile);
async function snapshot(root, directory = "proto") {
  const files = {};
  for (const entry of await readdir(join(root, directory), {
    withFileTypes: true,
  })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) Object.assign(files, await snapshot(root, path));
    else if (entry.isFile() && entry.name.endsWith(".pb.go")) {
      files[path] = (await readFile(join(root, path))).toString("base64");
    }
  }
  return files;
}

export async function checkGeneration(
  root,
  generate = () =>
    run("make", ["proto"], { cwd: root, maxBuffer: 8 * 1024 * 1024 }),
) {
  const before = await snapshot(root);
  if (!Object.keys(before).length)
    throw new Error("no generated outputs found");
  await generate();
  const after = await snapshot(root);
  const paths = new Set([...Object.keys(before), ...Object.keys(after)]);
  const changed = [...paths].filter((path) => before[path] !== after[path]);
  if (changed.length)
    throw new Error(`generated outputs differ: ${changed.join(", ")}`);
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  await checkGeneration(
    fileURLToPath(new URL("../../pluginsdk", import.meta.url)),
  );
  console.log("Generated protocol outputs match.");
}
