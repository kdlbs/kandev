import { execFile } from "node:child_process";
import { promisify, parseArgs } from "node:util";
import { mkdtemp, rm, appendFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { SourceRepository } from "./sdk-source.mjs";
import { ReleaseIO } from "./sdk-release-io.mjs";
import { assertReleaseContext } from "./sdk-runtime.mjs";
import {
  preparePublication,
  publishPublication,
} from "./sdk-publication-state.mjs";
import { PHASES, SdkDiagnostics } from "./sdk-diagnostics.mjs";
import { verifyTools } from "../pluginsdk/pack-candidate.mjs";
const exec = promisify(execFile);
const BOOTSTRAP_INSTRUCTIONS =
  "Verify the retained receipt signature and archive integrity. Publish that exact archive using the maintainer bootstrap instructions in docs/public/release-process.md, configure trusted publishing, then rerun the Stable backfill. No SDK completion has been recorded.";

export async function writeReleaseSummary(summary, { env = process.env } = {}) {
  const text = JSON.stringify(summary, null, 2);
  if (env.GITHUB_STEP_SUMMARY)
    await appendFile(
      env.GITHUB_STEP_SUMMARY,
      `\n### Plugin SDK\n\n\`\`\`json\n${text}\n\`\`\`\n`,
    );
  if (env.GITHUB_OUTPUT)
    await appendFile(env.GITHUB_OUTPUT, `status=${summary.status}\n`);
  console.log(JSON.stringify(summary));
}

function buildSummary(diagnostics, { result, source, failure }) {
  const status = failure ? "failed" : result?.status ?? "verified";
  const summary = diagnostics.summary({ status, result, failure });
  if (source) {
    summary.sdk_version = source.record.sdk_version;
    summary.host_release = source.record.host_release;
  }
  if (result?.status === "bootstrap_required") {
    summary.integrity = result.candidate.integrity;
    summary.assets = result.candidate.retention.urls;
    summary.instructions = BOOTSTRAP_INSTRUCTIONS;
  }
  return summary;
}

export async function runRelease({
  mode,
  hostTag,
  root = process.cwd(),
  env = process.env,
  makeRepository = (options) => new SourceRepository(options),
  makeDirectory = () => mkdtemp(join(tmpdir(), "kandev-sdk-release-")),
  removeDirectory = (directory) =>
    rm(directory, { recursive: true, force: true }),
  summaryWriter = writeReleaseSummary,
  verifyToolsImpl = verifyTools,
  execFileImpl = exec,
  diagnosticsLog = console.log,
  diagnosticsError = console.error,
} = {}) {
  if (!/^v\d+\.\d+\.\d+$/.test(hostTag ?? ""))
    throw new Error("--host-tag requires an exact Stable tag");
  const diagnostics = new SdkDiagnostics({ hostTag, log: diagnosticsLog });
  let directory = null;
  let result = null;
  let source = null;
  let packConfig = null;
  let failure = null;
  try {
    if (!["prepare", "publish", "verify", "tools"].includes(mode))
      throw new Error("invalid SDK release mode");
    if (mode === "prepare" || mode === "publish") assertReleaseContext(env);
    const fingerprint = env.RELEASE_GPG_FINGERPRINT;
    if (!/^[A-F0-9]{40}$/.test(fingerprint ?? ""))
      throw new Error("approved RELEASE_GPG_FINGERPRINT is required");
    await execFileImpl(
      "gpg",
      ["--batch", "--import", join(root, ".github/release-signing-key.asc")],
      { cwd: root, env },
    );
    const repository = makeRepository({ root, fingerprint, env });
    source = await repository.loadSource(hostTag);
    diagnostics.setSdkVersion(source.record.sdk_version);
    if (mode === "tools") {
      if (
        !/^\d+\.\d+\.\d+$/.test(source.packConfig.node) ||
        !/^\d+\.\d+\.\d+$/.test(source.packConfig.npm)
      )
        throw new Error("invalid original packaging tools");
      if (env.GITHUB_OUTPUT)
        await appendFile(
          env.GITHUB_OUTPUT,
          `node=${source.packConfig.node}\nnpm=${source.packConfig.npm}\nsdk_version=${source.record.sdk_version}\n`,
        );
      packConfig = source.packConfig;
      for (const phase of PHASES)
        diagnostics.phase(phase, "skipped", { reason: "tools_mode" });
    } else {
      diagnostics.start("source_verification");
      if (
        (mode === "prepare" || mode === "publish") &&
        (await repository.text("rev-parse", "HEAD")) !== source.currentCommit
      )
        throw new Error("checked-out release differs from signed host tag");
      verifyToolsImpl(
        source.packConfig,
        process.version,
        (await execFileImpl("npm", ["--version"])).stdout.trim(),
      );
      directory = await makeDirectory();
      const io = new ReleaseIO({
        root,
        directory,
        repository,
        source,
        env,
        diagnostics,
      });
      const context = {
        source,
        fingerprint,
        firstPublishOIDCReady: env.SDK_FIRST_PUBLISH_OIDC_READY === "true",
      };
      if (mode === "prepare") result = await preparePublication(io, context);
      else if (mode === "publish")
        result = await publishPublication(io, context);
      else {
        diagnostics.start("candidate_verification");
        const tag = await io.readTag(source);
        if (
          !tag ||
          tag.commit !== source.commit ||
          tag.recordDigest !== source.recordDigest
        )
          throw new Error("SDK tag identity conflict");
        diagnostics.phase("source_verification", "passed", {
          source_commit: source.commit,
          host_tag: source.hostTag,
        });
        const candidate = await io.obtainCandidate(source, {
          allowCreate: false,
        });
        await io.verifyFinalizedCandidate(candidate, source);
        diagnostics.phase("candidate_verification", "passed", {
          integrity: candidate.integrity,
        });
        const registry = await io.readRegistry(source.record.sdk_version);
        if (
          !registry.version ||
          registry.version.integrity !== candidate.integrity ||
          registry.version.recordDigest !== source.recordDigest
        )
          throw new Error("public SDK identity conflict");
        if (
          !registry.version.provenance &&
          !candidate.receipt.bootstrap_allowed
        )
          throw new Error("required npm provenance is absent");
        diagnostics.phase("version_visibility", "passed", {
          observed_version: registry.version?.version ?? null,
          observed_latest: registry.latest ?? null,
        });
        diagnostics.phase("channel_promotion", "skipped", {
          reason: "verify_mode",
        });
        diagnostics.setPublication("confirmed");
        const origin = registry.version.provenance
          ? "oidc"
          : "manual_bootstrap";
        diagnostics.start("public_consumers");
        await io.verifyConsumers(candidate, origin);
        diagnostics.phase("public_consumers", "passed", { origin });
        result = { status: "verified", origin };
      }
    }
  } catch (error) {
    failure = error;
  }
  if (failure) diagnostics.fail(failure);
  let summary = null;
  if (mode !== "tools" || failure) {
    summary = buildSummary(diagnostics, { result, source, failure });
    try {
      await summaryWriter(summary, { env });
    } catch {
      diagnosticsError(
        "SDK release summary could not be written; the original release result is unchanged.",
      );
    }
  }
  if (directory) {
    try {
      await removeDirectory(directory);
    } catch {
      diagnosticsError(
        "SDK release temporary directory cleanup failed; the original release result is unchanged.",
      );
    }
  }
  if (failure) throw failure;
  if (mode === "tools") return packConfig;
  if (result?.status === "bootstrap_required") process.exitCode = 2;
  return summary;
}
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  const { values } = parseArgs({
    options: { mode: { type: "string" }, "host-tag": { type: "string" } },
  });
  await runRelease({ mode: values.mode, hostTag: values["host-tag"] });
}
