import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { digest, verifiedFingerprint } from "./sdk-artifact.mjs";
import { planSDKRelease, recordPath, validateRecord } from "./plugin-sdk.mjs";

const exec = promisify(execFile);
const tagPattern =
  /^(?:pluginsdk\/)?v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/;
export class SourceRepository {
  constructor({ root, fingerprint, env = process.env }) {
    this.root = root;
    this.fingerprint = fingerprint;
    this.env = env;
  }
  async git(...args) {
    return exec("git", args, {
      cwd: this.root,
      env: this.env,
      maxBuffer: 16 * 1024 * 1024,
      timeout: 120_000,
    });
  }
  async text(...args) {
    return (await this.git(...args)).stdout.trim();
  }
  async file(commit, path) {
    return (await this.git("show", `${commit}:${path}`)).stdout;
  }
  async readSignedTag(tag) {
    if (!tagPattern.test(tag)) throw new Error("invalid release tag");
    try {
      await this.git("ls-remote", "--exit-code", "origin", `refs/tags/${tag}`);
    } catch (error) {
      if (error.code === 2) return null;
      throw error;
    }
    await this.git("fetch", "--no-tags", "origin", `refs/tags/${tag}`);
    const object = await this.text("rev-parse", "FETCH_HEAD");
    const status = await this.git("verify-tag", "--raw", object);
    const fingerprint = verifiedFingerprint(status.stderr, this.fingerprint);
    const commit = await this.text("rev-parse", `${object}^{commit}`);
    return { commit, fingerprint, object };
  }
  async loadSource(hostTag) {
    if (hostTag.startsWith("pluginsdk/"))
      throw new Error("expected a root host tag");
    const current = await this.readSignedTag(hostTag);
    if (!current) throw new Error("signed host release tag is absent");
    const raw = await this.file(current.commit, recordPath);
    const record = JSON.parse(raw);
    validateRecord(record);
    const originTag = `v${record.host_release}`;
    const original =
      originTag === hostTag ? current : await this.readSignedTag(originTag);
    if (!original) throw new Error("originating signed host tag is absent");
    const originalRaw = await this.file(original.commit, recordPath);
    const paths = (
      await this.git(
        "diff",
        "--name-only",
        "-z",
        original.commit,
        current.commit,
        "--",
      )
    ).stdout
      .split("\0")
      .filter(Boolean);
    planSDKRelease(record, JSON.parse(originalRaw), paths, hostTag.slice(1));
    if (raw !== originalRaw)
      throw new Error("reused SDK record bytes differ from original source");
    const packConfig = JSON.parse(
      await this.file(original.commit, "scripts/pluginsdk/pack-config.json"),
    );
    return {
      commit: original.commit,
      currentCommit: current.commit,
      hostTag: originTag,
      currentHostTag: hostTag,
      sdkTag: `pluginsdk/v${record.sdk_version}`,
      record,
      recordRaw: raw,
      recordDigest: digest(raw),
      packConfig,
    };
  }
  async createSDKTag(source) {
    const existing = await this.readSDKTag(source);
    if (existing) {
      if (
        existing.commit !== source.commit ||
        existing.recordDigest !== source.recordDigest
      )
        throw new Error("SDK tag conflicts with original source");
      return;
    }
    let local;
    try {
      local = await this.text(
        "rev-parse",
        "--verify",
        `refs/tags/${source.sdkTag}`,
      );
    } catch (error) {
      if (error.code !== 128) throw error;
    }
    if (local) {
      const status = await this.git("verify-tag", "--raw", local);
      verifiedFingerprint(status.stderr, this.fingerprint);
      if ((await this.text("rev-parse", `${local}^{commit}`)) !== source.commit)
        throw new Error("local SDK tag conflicts with original source");
    } else
      await this.git(
        "tag",
        "-s",
        "-u",
        this.fingerprint,
        "-m",
        `Plugin SDK ${source.record.sdk_version}`,
        source.sdkTag,
        source.commit,
      );
    await this.git(
      "-c",
      "credential.helper=",
      "-c",
      "credential.helper=!gh auth git-credential",
      "push",
      "origin",
      `refs/tags/${source.sdkTag}`,
    );
  }
  async signReceipt(receipt, signature) {
    await exec(
      "gpg",
      [
        "--batch",
        "--yes",
        "--armor",
        "--local-user",
        this.fingerprint,
        "--output",
        signature,
        "--detach-sign",
        receipt,
      ],
      { cwd: this.root, env: this.env, timeout: 30_000 },
    );
  }
  async verifyReceipt(receipt, signature) {
    const result = await exec(
      "gpg",
      ["--batch", "--status-fd=1", "--verify", signature, receipt],
      { cwd: this.root, env: this.env, timeout: 30_000 },
    );
    return verifiedFingerprint(result.stdout, this.fingerprint);
  }
  async readSDKTag(source) {
    const tag = await this.readSignedTag(source.sdkTag);
    if (!tag) return null;
    return {
      ...tag,
      recordDigest: digest(await this.file(tag.commit, recordPath)),
    };
  }
}
