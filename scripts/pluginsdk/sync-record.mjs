import { readFile, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export async function syncRecord(root, check = false) {
  const canonical = await readFile(
    join(root, "pluginsdk/sdk-release.json"),
    "utf8",
  );
  const record = JSON.parse(canonical);
  const directory = join(root, "apps/packages/plugin-sdk");
  const manifest = JSON.parse(
    await readFile(join(directory, "package.json"), "utf8"),
  );
  if (manifest.version !== record.sdk_version)
    throw new Error("package version differs from canonical SDK version");
  const target = join(directory, "sdk-release.json");
  if (check) {
    if ((await readFile(target, "utf8")) !== canonical)
      throw new Error("npm release record differs from canonical record");
  } else {
    await writeFile(target, canonical);
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  await syncRecord(
    fileURLToPath(new URL("../../", import.meta.url)),
    process.argv.includes("--check"),
  );
}
