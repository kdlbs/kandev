import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { assetNames, digest } from "./sdk-artifact.mjs";

const execute = promisify(execFile);
const repo = "kdlbs/kandev";
const fileKey = (key) =>
  ({ receipt: "receiptPath", signature: "signaturePath" })[key] ?? key;
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
  async json(endpoint) {
    return JSON.parse((await this.gh(["api", endpoint])).stdout);
  }
  async readSDK(id, source) {
    const release = this.validate(
      await this.json(`repos/${repo}/releases/${id}`),
      source,
      true,
    );
    if (release.id !== id)
      throw new Error("SDK release identity changed during recovery");
    return release;
  }
  validate(release, source, sdk) {
    if (
      !release ||
      !Number.isSafeInteger(release.id) ||
      release.id <= 0 ||
      release.tag_name !== (sdk ? source.sdkTag : source.hostTag) ||
      typeof release.draft !== "boolean" ||
      typeof release.immutable !== "boolean" ||
      release.prerelease !== false ||
      !Array.isArray(release.assets) ||
      (sdk && release.target_commitish !== source.commit)
    )
      throw new Error("SDK evidence release identity or target conflict");
    const names = new Set();
    for (const asset of release.assets) {
      if (!asset || typeof asset.name !== "string" || names.has(asset.name))
        throw new Error("duplicate or invalid SDK release asset");
      names.add(asset.name);
    }
    return release;
  }
  async findSDK(source) {
    const matches = new Map();
    for (let page = 1; ; page++) {
      const releases = await this.json(
        `repos/${repo}/releases?per_page=100&page=${page}`,
      );
      if (!Array.isArray(releases))
        throw new Error("invalid SDK release listing");
      for (const release of releases)
        if (release?.tag_name === source.sdkTag)
          matches.set(release.id, release);
      if (releases.length < 100) break;
    }
    if (matches.size > 1) throw new Error("ambiguous duplicate SDK releases");
    if (!matches.size) return null;
    const match = [...matches.values()][0];
    this.validate(match, source, true);
    return this.readSDK(match.id, source);
  }
  entries(release, source) {
    const names = new Set(Object.values(assetNames(source.record.sdk_version)));
    return new Map(
      release?.assets
        .filter((asset) => names.has(asset.name))
        .map((asset) => [asset.name, asset]) ?? [],
    );
  }
  async bytes(asset) {
    if (
      !Number.isSafeInteger(asset.id) ||
      asset.id <= 0 ||
      !Number.isSafeInteger(asset.size) ||
      asset.size <= 0 ||
      !/^sha256:[a-f0-9]{64}$/.test(asset.digest ?? "")
    )
      throw new Error("invalid SDK asset identity");
    const bytes = Buffer.from(
      (
        await this.gh(
          [
            "api",
            `repos/${repo}/releases/assets/${asset.id}`,
            "-H",
            "Accept: application/octet-stream",
          ],
          true,
        )
      ).stdout,
    );
    if (
      bytes.length !== asset.size ||
      `sha256:${digest(bytes)}` !== asset.digest
    )
      throw new Error(`SDK retained asset integrity conflict: ${asset.name}`);
    return bytes;
  }
  async locations(source) {
    const sdk = await this.findSDK(source);
    const host = this.validate(
      await this.json(
        `repos/${repo}/releases/tags/${encodeURIComponent(source.hostTag)}`,
      ),
      source,
      false,
    );
    const current = this.entries(sdk, source);
    const legacy = this.entries(host, source);
    if (sdk && !sdk.draft && current.size !== 3)
      throw new Error("published SDK evidence is incomplete");
    if (current.size && legacy.size) {
      for (const [name, asset] of current)
        if (
          legacy.has(name) &&
          digest(await this.bytes(asset)) !==
            digest(await this.bytes(legacy.get(name)))
        )
          throw new Error(`SDK evidence locations conflict: ${name}`);
      if (current.size !== 3 && legacy.size !== 3)
        throw new Error("ambiguous partial SDK evidence locations");
    }
    const useLegacy =
      (legacy.size === 3 && current.size !== 3) ||
      (!current.size && legacy.size > 0);
    return {
      sdk,
      release: useLegacy ? host : sdk,
      kind: useLegacy ? "legacy" : "sdk",
      entries: useLegacy ? legacy : current,
    };
  }
  retention(location) {
    const urls = [];
    for (const asset of location.release?.draft
      ? []
      : location.entries.values()) {
      const url = new URL(asset.browser_download_url);
      if (
        url.origin !== "https://github.com" ||
        url.username ||
        url.password ||
        decodeURIComponent(url.pathname) !==
          `/${repo}/releases/download/${location.release.tag_name}/${asset.name}`
      )
        throw new Error("invalid SDK asset download URL");
      urls.push(asset.browser_download_url);
    }
    return {
      kind: location.kind,
      releaseId: location.release?.id,
      tag: location.release?.tag_name,
      draft: location.release?.draft,
      urls,
    };
  }
  async download(source, names, directory = this.directory) {
    await mkdir(directory, { recursive: true });
    const location = await this.locations(source);
    const result = { retention: this.retention(location) };
    for (const [key, name] of Object.entries(names)) {
      const asset = location.entries.get(name);
      if (!asset) continue;
      const path = join(directory, name);
      await writeFile(path, await this.bytes(asset));
      result[fileKey(key)] = path;
    }
    return result;
  }
  async write(method, endpoint, payload, name) {
    await mkdir(this.directory, { recursive: true });
    const path = join(this.directory, name);
    await writeFile(path, JSON.stringify(payload));
    return this.gh(["api", endpoint, "--method", method, "--input", path]);
  }
  async observe(read, valid, label, writeError) {
    for (let attempt = 0; attempt < 3; attempt++) {
      const value = await read();
      if (valid(value)) return value;
      if (attempt < 2) await this.wait(attempt);
    }
    throw new Error(
      `${label} did not become visible${writeError ? ": " + writeError.message : ""}`,
    );
  }
  async ensureDraft(source, sdk) {
    if (sdk) return sdk;
    let writeError;
    try {
      await this.write(
        "POST",
        `repos/${repo}/releases`,
        {
          tag_name: source.sdkTag,
          target_commitish: source.commit,
          name: `Plugin SDK ${source.record.sdk_version}`,
          draft: true,
          prerelease: false,
          make_latest: "false",
        },
        "sdk-draft.json",
      );
    } catch (error) {
      writeError = error;
    }
    return this.observe(
      () => this.findSDK(source),
      (value) => value !== null,
      "SDK draft",
      writeError,
    );
  }
  async verifyBytes(candidate, source, location, complete = false) {
    const names = assetNames(source.record.sdk_version);
    for (const [key, name] of Object.entries(names)) {
      const asset = location.entries.get(name);
      if (!asset) {
        if (complete) throw new Error(`SDK evidence is incomplete: ${name}`);
        continue;
      }
      if (
        digest(await readFile(candidate[fileKey(key)])) !==
        digest(await this.bytes(asset))
      )
        throw new Error(`retained SDK asset conflict: ${name}`);
    }
  }
  async retain(candidate, source) {
    let location = await this.locations(source);
    await this.verifyBytes(candidate, source, location);
    if (location.entries.size === 3) {
      candidate.retention = this.retention(location);
      return;
    }
    const sdk = await this.ensureDraft(source, location.sdk);
    if (!sdk.draft || sdk.immutable)
      throw new Error("published SDK evidence is incomplete");
    location = {
      sdk,
      release: sdk,
      kind: "sdk",
      entries: this.entries(sdk, source),
    };
    await this.verifyBytes(candidate, source, location);
    for (const [key, name] of Object.entries(
      assetNames(source.record.sdk_version),
    )) {
      if (location.entries.has(name)) continue;
      let writeError;
      try {
        await this.gh([
          "api",
          `https://uploads.github.com/repos/${repo}/releases/${sdk.id}/assets?name=${encodeURIComponent(name)}`,
          "--method",
          "POST",
          "-H",
          "Content-Type: application/octet-stream",
          "--input",
          candidate[fileKey(key)],
        ]);
      } catch (error) {
        writeError = error;
      }
      const updated = await this.observe(
        () => this.readSDK(sdk.id, source),
        (value) => value.assets.some((asset) => asset.name === name),
        `retained asset ${name}`,
        writeError,
      );
      location = {
        sdk: updated,
        release: updated,
        kind: "sdk",
        entries: this.entries(updated, source),
      };
      await this.verifyBytes(candidate, source, location);
    }
    await this.verifyBytes(candidate, source, location, true);
    candidate.retention = this.retention(location);
  }
  async finalize(candidate, source) {
    let location = await this.locations(source);
    this.verifyReleaseIdentity(candidate, location);
    await this.verifyBytes(candidate, source, location, true);
    if (location.kind === "sdk" && location.release.draft) {
      const id = location.release.id;
      let writeError;
      try {
        await this.write(
          "PATCH",
          `repos/${repo}/releases/${id}`,
          { draft: false, prerelease: false, make_latest: "false" },
          "sdk-publish.json",
        );
      } catch (error) {
        writeError = error;
      }
      const published = await this.observe(
        () => this.readSDK(id, source),
        (value) => !value.draft,
        "published SDK release",
        writeError,
      );
      location = {
        sdk: published,
        release: published,
        kind: "sdk",
        entries: this.entries(published, source),
      };
      await this.verifyBytes(candidate, source, location, true);
    }
    if (location.release.draft)
      throw new Error("candidate evidence is still a draft");
    candidate.retention = this.retention(location);
  }
  async verifyFinalized(candidate, source) {
    const location = await this.locations(source);
    this.verifyReleaseIdentity(candidate, location);
    if (!location.release || location.release.draft)
      throw new Error("SDK candidate evidence is not finalized");
    await this.verifyBytes(candidate, source, location, true);
    candidate.retention = this.retention(location);
  }
  verifyReleaseIdentity(candidate, location) {
    if (
      !candidate.retention?.releaseId ||
      candidate.retention.releaseId !== location.release?.id
    )
      throw new Error("SDK release identity changed during recovery");
  }
}
