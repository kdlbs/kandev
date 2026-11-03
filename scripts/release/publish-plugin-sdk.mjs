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
import { verifyTools } from "../pluginsdk/pack-candidate.mjs";
import { assetNames } from "./sdk-artifact.mjs";
const exec = promisify(execFile);
export async function runRelease({
  mode,
  hostTag,
  root = process.cwd(),
  env = process.env,
}) {
  if (!["prepare", "publish", "verify", "tools"].includes(mode))
    throw new Error("invalid SDK release mode");
  if (!/^v\d+\.\d+\.\d+$/.test(hostTag ?? ""))
    throw new Error("--host-tag requires an exact Stable tag");
  if (mode === "prepare" || mode === "publish") assertReleaseContext(env);
  const fingerprint = env.RELEASE_GPG_FINGERPRINT;
  if (!/^[A-F0-9]{40}$/.test(fingerprint ?? ""))
    throw new Error("approved RELEASE_GPG_FINGERPRINT is required");
  await exec(
    "gpg",
    ["--batch", "--import", join(root, ".github/release-signing-key.asc")],
    { cwd: root, env },
  );
  const repository = new SourceRepository({ root, fingerprint, env });
  const source = await repository.loadSource(hostTag);
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
    return source.packConfig;
  }
  if (
    (mode === "prepare" || mode === "publish") &&
    (await repository.text("rev-parse", "HEAD")) !== source.currentCommit
  )
    throw new Error("checked-out release differs from signed host tag");
  verifyTools(
    source.packConfig,
    process.version,
    (await exec("npm", ["--version"])).stdout.trim(),
  );
  const directory = await mkdtemp(join(tmpdir(), "kandev-sdk-release-"));
  try {
    const io = new ReleaseIO({ root, directory, repository, source, env });
    const context = {
      source,
      fingerprint,
      firstPublishOIDCReady: env.SDK_FIRST_PUBLISH_OIDC_READY === "true",
    };
    let result;
    if (mode === "prepare") result = await preparePublication(io, context);
    else if (mode === "publish") result = await publishPublication(io, context);
    else {
      const tag = await io.readTag(source);
      if (
        !tag ||
        tag.commit !== source.commit ||
        tag.recordDigest !== source.recordDigest
      )
        throw new Error("SDK tag identity conflict");
      const candidate = await io.obtainCandidate(source, {
        allowCreate: false,
      });
      const registry = await io.readRegistry(source.record.sdk_version);
      if (
        !registry.version ||
        registry.version.integrity !== candidate.integrity ||
        registry.version.recordDigest !== source.recordDigest
      )
        throw new Error("public SDK identity conflict");
      if (!registry.version.provenance && !candidate.receipt.bootstrap_allowed)
        throw new Error("required npm provenance is absent");
      const origin = registry.version.provenance ? "oidc" : "manual_bootstrap";
      await io.verifyConsumers(candidate, origin);
      result = { status: "verified", origin };
    }
    const summary = {
      status: result.status,
      sdk_version: source.record.sdk_version,
      host_release: source.record.host_release,
      origin: result.origin,
    };
    if (result.status === "bootstrap_required") {
      const names = assetNames(source.record.sdk_version);
      summary.integrity = result.candidate.integrity;
      summary.assets = Object.values(names).map(
        (name) =>
          `https://github.com/kdlbs/kandev/releases/download/${source.hostTag}/${name}`,
      );
      summary.instructions =
        "Verify the retained receipt signature and archive integrity. Publish that exact archive using the maintainer bootstrap instructions in docs/public/release-process.md, configure trusted publishing, then rerun the Stable backfill. No SDK completion has been recorded.";
    }
    if (env.GITHUB_STEP_SUMMARY)
      await appendFile(
        env.GITHUB_STEP_SUMMARY,
        `\n### Plugin SDK\n\n\`\`\`json\n${JSON.stringify(summary, null, 2)}\n\`\`\`\n`,
      );
    if (env.GITHUB_OUTPUT)
      await appendFile(env.GITHUB_OUTPUT, `status=${result.status}\n`);
    console.log(JSON.stringify(summary));
    if (result.status === "bootstrap_required") process.exitCode = 2;
    return summary;
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
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
