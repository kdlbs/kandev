import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { parseArgs, promisify } from "node:util";

const run = promisify(execFile);
export const packConfig = JSON.parse(
  await readFile(new URL("./pack-config.json", import.meta.url), "utf8"),
);
export function verifyTools(config, node, npm) {
  if (node !== `v${config.node}`)
    throw new Error(`Node ${config.node} is required for SDK packing`);
  if (npm !== config.npm)
    throw new Error(`npm ${config.npm} is required for SDK packing`);
}
export function validateInventory(files, config = packConfig) {
  if (
    JSON.stringify([...files].sort()) !==
    JSON.stringify([...config.files].sort())
  ) {
    throw new Error(
      `SDK archive inventory differs from allowlist: ${files.join(", ")}`,
    );
  }
}
export async function integrityOf(archive) {
  return (
    "sha512-" +
    createHash("sha512")
      .update(await readFile(archive))
      .digest("base64")
  );
}
export async function inspectArchive(archive, config = packConfig) {
  const { stdout } = await run(
    "python3",
    [
      "-c",
      `
import json, sys, tarfile
with tarfile.open(sys.argv[1], 'r:gz') as archive:
 files = {}
 for member in archive.getmembers():
  if not member.isfile() or not member.name.startswith('package/'): raise ValueError('invalid SDK archive entry')
  name = member.name[len('package/'):]
  if name in files or '..' in name.split('/'): raise ValueError('invalid SDK archive path')
  files[name] = archive.extractfile(member).read().decode('utf-8')
 print(json.dumps(files))
`,
      archive,
    ],
    { maxBuffer: 8 * 1024 * 1024 },
  );
  const files = JSON.parse(stdout);
  validateInventory(Object.keys(files), config);
  const manifest = JSON.parse(files["package.json"]);
  const record = JSON.parse(files["sdk-release.json"]);
  if (
    manifest.name !== "@kdlbs/plugin-sdk" ||
    manifest.version !== record.sdk_version
  )
    throw new Error("SDK package identity differs from release record");
  for (const key of [
    "dependencies",
    "peerDependencies",
    "optionalDependencies",
  ]) {
    if (Object.keys(manifest[key] ?? {}).length)
      throw new Error(`SDK must have no ${key}`);
  }
  if (/(?:from\s+|import\s*(?:\(\s*)?)["'](?![./])/.test(files["src/index.ts"]))
    throw new Error("SDK source contains an external import");
  return {
    files: Object.keys(files).sort(),
    record,
    recordDigest: createHash("sha256")
      .update(files["sdk-release.json"])
      .digest("hex"),
  };
}
export async function packCandidate({
  directory,
  output,
  config = packConfig,
}) {
  const npm = (await run("npm", ["--version"])).stdout.trim();
  verifyTools(config, process.version, npm);
  await mkdir(output, { recursive: true });
  const { stdout } = await run(
    "npm",
    [
      "pack",
      "--json",
      "--ignore-scripts",
      "--pack-destination",
      resolve(output),
    ],
    { cwd: directory },
  );
  const [packed] = JSON.parse(stdout);
  const archive = join(resolve(output), packed.filename);
  const { files, record, recordDigest } = await inspectArchive(archive, config);
  const integrity = await integrityOf(archive);
  if (integrity !== packed.integrity)
    throw new Error("npm pack integrity differs from artifact bytes");
  return {
    archive,
    files,
    integrity,
    recordDigest,
    node: config.node,
    npm,
    sdk_version: record.sdk_version,
  };
}
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  const { values } = parseArgs({
    options: {
      directory: {
        type: "string",
        default: fileURLToPath(
          new URL("../../apps/packages/plugin-sdk", import.meta.url),
        ),
      },
      output: { type: "string" },
    },
  });
  if (!values.output) throw new Error("--output is required");
  console.log(JSON.stringify(await packCandidate(values)));
}
