import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, readFile, writeFile, copyFile } from "node:fs/promises";
import { join } from "node:path";
import { assetNames, digest } from "./sdk-artifact.mjs";
const execute = promisify(execFile);
const repo = "kdlbs/kandev";
export class ReleaseAssets {
  constructor({
    root,
    directory,
    run = execute,
    wait = (attempt) =>
      new Promise((resolve) => setTimeout(resolve, 1000 * (attempt + 1))),
  }) {
    Object.assign(this, { root, directory, run, wait });
  }
  async gh(args, binary = false) {
    return this.run("gh", args, {
      cwd: this.root,
      encoding: binary ? "buffer" : "utf8",
      maxBuffer: 32 * 1024 * 1024,
      timeout: 120_000,
    });
  }
  async index(tag) {
    const release = JSON.parse(
      (
        await this.gh([
          "api",
          `repos/${repo}/releases/tags/${encodeURIComponent(tag)}`,
        ])
      ).stdout,
    );
    if (!Array.isArray(release.assets))
      throw new Error("invalid host release asset list");
    const names = new Set();
    for (const asset of release.assets) {
      if (names.has(asset.name))
        throw new Error("duplicate host release asset");
      names.add(asset.name);
    }
    return release.assets;
  }
  async bytes(asset) {
    return (
      await this.gh(
        [
          "api",
          `repos/${repo}/releases/assets/${asset.id}`,
          "-H",
          "Accept: application/octet-stream",
        ],
        true,
      )
    ).stdout;
  }
  async download(tag, names, directory = this.directory) {
    await mkdir(directory, { recursive: true });
    const assets = await this.index(tag);
    const result = {};
    for (const [key, name] of Object.entries(names)) {
      const asset = assets.find((item) => item.name === name);
      if (!asset) continue;
      const path = join(directory, name);
      await writeFile(path, await this.bytes(asset));
      result[
        { receipt: "receiptPath", signature: "signaturePath" }[key] ?? key
      ] = path;
    }
    return result;
  }
  async retain(candidate, source) {
    const names = assetNames(source.record.sdk_version);
    for (const [key, name] of Object.entries(names)) {
      const path =
        candidate[
          { receipt: "receiptPath", signature: "signaturePath" }[key] ?? key
        ];
      const expected = digest(await readFile(path));
      const verify = async () => {
        const asset = (await this.index(source.hostTag)).find(
          (item) => item.name === name,
        );
        if (!asset) return false;
        if (digest(await this.bytes(asset)) !== expected)
          throw new Error(`retained asset conflict: ${name}`);
        return true;
      };
      if (await verify()) continue;
      const uploadDirectory = join(this.directory, "uploads");
      await mkdir(uploadDirectory, { recursive: true });
      const uploadPath = join(uploadDirectory, name);
      await copyFile(path, uploadPath);
      let writeError;
      try {
        await this.gh([
          "release",
          "upload",
          source.hostTag,
          uploadPath,
          "--repo",
          repo,
        ]);
      } catch (error) {
        writeError = error;
      }
      let visible = false;
      for (let attempt = 0; attempt < 3; attempt++) {
        if (await verify()) {
          visible = true;
          break;
        }
        if (attempt < 2) await this.wait(attempt);
      }
      if (!visible)
        throw new Error(
          `retained asset did not become visible: ${name}${writeError ? ": " + writeError.message : ""}`,
        );
    }
  }
}
