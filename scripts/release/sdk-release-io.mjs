import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { CandidateStore } from "./sdk-candidate-store.mjs";
import { ReleaseAssets } from "./sdk-assets.mjs";
import {
  readPackage,
  downloadArchive,
  publicRegistryURL,
} from "./sdk-registry.mjs";
import { packCandidate, inspectArchive } from "../pluginsdk/pack-candidate.mjs";
import { smokeGo } from "../pluginsdk/consumer-smoke.mjs";
import { publicationEnvironment } from "./sdk-runtime.mjs";
import { digest } from "./sdk-artifact.mjs";
const exec = promisify(execFile);

export class ReleaseIO {
  constructor({ root, directory, repository, source, env = process.env }) {
    Object.assign(this, { root, directory, repository, source, env });
    this.smoke = smokeGo;
    this.assets = new ReleaseAssets({
      root,
      directory: join(directory, "retained"),
    });
    this.store = new CandidateStore({
      directory: join(directory, "candidate"),
      assets: this.assets,
      workflow: {
        workflow_ref: env.GITHUB_WORKFLOW_REF,
        workflow_sha: env.GITHUB_WORKFLOW_SHA,
        run_id: env.GITHUB_RUN_ID,
        run_attempt: env.GITHUB_RUN_ATTEMPT,
      },
      build: (source) => this.build(source),
      sign: (...args) => repository.signReceipt(...args),
      verify: (...args) => repository.verifyReceipt(...args),
      verifyWorkflow: (receipt) => this.verifyWorkflow(receipt),
    });
  }
  wait(attempt) {
    return new Promise((resolve) =>
      setTimeout(resolve, 10_000 * (attempt + 1)),
    );
  }
  readTag(source) {
    return this.repository.readSDKTag(source);
  }
  obtainCandidate(source, options) {
    return this.store.obtain(source, options);
  }
  retainCandidate(candidate, source) {
    return this.assets.retain(candidate, source);
  }
  createTag(source) {
    return this.repository.createSDKTag(source);
  }
  async verifyWorkflow(receipt) {
    const { stdout } = await exec(
      "gh",
      [
        "api",
        `repos/kdlbs/kandev/actions/runs/${receipt.run_id}/attempts/${receipt.run_attempt}`,
      ],
      { cwd: this.root },
    );
    const run = JSON.parse(stdout);
    if (
      run.repository?.full_name !== "kdlbs/kandev" ||
      run.path !== ".github/workflows/release.yml" ||
      run.event !== "workflow_dispatch" ||
      run.head_branch !== "main" ||
      run.head_sha !== receipt.workflow_sha
    )
      throw new Error("unsigned receipt workflow cannot be authenticated");
  }
  async build(source) {
    const tree = join(this.directory, "original-source");
    await mkdir(tree, { recursive: true });
    const archive = join(this.directory, "original-source.tar");
    await this.repository.git(
      "archive",
      "--format=tar",
      "-o",
      archive,
      source.commit,
      "pluginsdk",
      "apps/packages/plugin-sdk",
      "scripts/pluginsdk/pack-config.json",
    );
    await exec("tar", ["-xf", archive, "-C", tree]);
    const candidate = await packCandidate({
      directory: join(tree, "apps/packages/plugin-sdk"),
      output: join(this.directory, "packed"),
      config: source.packConfig,
    });
    if (candidate.recordDigest !== source.recordDigest)
      throw new Error("packed SDK record differs from signed source");
    await smokeGo({
      mode: "local",
      version: `v${source.record.sdk_version}`,
      frontend: true,
      sourceRoot: tree,
      candidate,
      packConfig: source.packConfig,
      expectedRecordDigest: source.recordDigest,
    });
    return candidate;
  }
  async readRegistry(version) {
    const state = await readPackage(version);
    if (!state.version) return state;
    const archive = join(this.directory, `registry-${version}.tgz`);
    await downloadArchive(
      state.version.dist.tarball,
      archive,
      state.version.dist.integrity,
    );
    // The inventory is tied to this SDK's original source, including on retries.
    const source =
      version === this.source.record.sdk_version
        ? this.source
        : await this.sourceForVersion(version);
    const inspected = await inspectArchive(archive, source.packConfig);
    const provenance = state.version.dist.attestations;
    if (provenance) {
      publicRegistryURL(provenance.url);
      if (
        provenance.provenance?.predicateType !==
        "https://slsa.dev/provenance/v1"
      )
        throw new Error("unsupported npm provenance");
    }
    return {
      ...state,
      version: {
        integrity: state.version.dist.integrity,
        recordDigest: inspected.recordDigest,
        provenance: !!provenance,
      },
    };
  }
  async sourceForVersion(version) {
    const tag = await this.repository.readSignedTag(`pluginsdk/v${version}`);
    if (!tag) throw new Error("newer latest has no authenticated SDK tag");
    const record = JSON.parse(
      await this.repository.file(tag.commit, "pluginsdk/sdk-release.json"),
    );
    const source = await this.repository.loadSource(`v${record.host_release}`);
    if (source.commit !== tag.commit || source.record.sdk_version !== version)
      throw new Error("newer latest SDK mapping conflict");
    return source;
  }
  async verifyNewerLatest(version) {
    const source = await this.sourceForVersion(version);
    const candidate = await this.obtainCandidate(source, {
      allowCreate: false,
    });
    const state = await this.readRegistry(version);
    if (
      !state.version ||
      state.version.integrity !== candidate.integrity ||
      state.version.recordDigest !== source.recordDigest ||
      !state.version.provenance
    )
      throw new Error("newer latest SDK identity conflict");
  }
  async npm(args) {
    const cwd = join(this.directory, "npm-publish");
    await mkdir(cwd, { recursive: true });
    await writeFile(
      join(cwd, "package.json"),
      JSON.stringify({ private: true }),
    );
    return exec("npm", args, {
      cwd,
      env: publicationEnvironment(cwd, this.env),
      timeout: 180_000,
      maxBuffer: 4 * 1024 * 1024,
    });
  }
  publish(candidate) {
    return this.npm([
      "publish",
      candidate.archive,
      "--provenance",
      "--access",
      "public",
      "--ignore-scripts",
      "--registry",
      "https://registry.npmjs.org",
      "--tag",
      "sdk-release",
    ]);
  }
  moveLatest(version) {
    return this.npm([
      "dist-tag",
      "add",
      `@kdlbs/plugin-sdk@${version}`,
      "latest",
      "--registry",
      "https://registry.npmjs.org",
    ]);
  }
  async verifyConsumers(candidate, origin) {
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        await this.smoke({
          mode: "public",
          version: `v${this.source.record.sdk_version}`,
          frontend: true,
          candidate,
          packConfig: this.source.packConfig,
          expectedRecordDigest: this.source.recordDigest,
        });
        break;
      } catch (error) {
        if (attempt === 2) throw error;
        await this.wait(attempt);
      }
    }
    console.log(
      JSON.stringify({
        status: "public_consumers_verified",
        origin,
        source_commit: this.source.commit,
        host_tag: this.source.hostTag,
        sdk_tag: this.source.sdkTag,
        integrity: candidate.integrity,
        receipt_digest: digest(await readFile(candidate.receiptPath)),
      }),
    );
  }
}
