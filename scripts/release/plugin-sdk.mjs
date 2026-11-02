import { execFile } from "node:child_process";
import { readFile, writeFile } from "node:fs/promises";
import { matchesGlob, join } from "node:path";
import { isDeepStrictEqual, promisify } from "node:util";

export const contractInputs = JSON.parse(
  await readFile(
    new URL("./plugin-sdk-contract-inputs.json", import.meta.url),
    "utf8",
  ),
);
export const stablePattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
export function stableParts(value) {
  if (typeof value !== "string" || !stablePattern.test(value))
    throw new Error(`expected exact Stable version, got ${value}`);
  return value.split(".").map(BigInt);
}
export function compareVersions(left, right) {
  const a = stableParts(left),
    b = stableParts(right);
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  return 0;
}
export function isContractInput(path) {
  return (
    contractInputs.include.some((pattern) => matchesGlob(path, pattern)) &&
    !contractInputs.exclude.some((pattern) => matchesGlob(path, pattern))
  );
}
export function validateRecord(record, draft = false) {
  if (record.schema_version !== 1)
    throw new Error("unsupported SDK record schema_version");
  stableParts(record.sdk_version);
  if (stableParts(record.sdk_version)[0] >= 2n)
    throw new Error(
      "SDK v2 requires an explicitly migrated Go major import path",
    );
  if (
    !["initial", "patch", "additive", "breaking"].includes(record.change_kind)
  )
    throw new Error("invalid SDK change_kind");
  if (
    !Array.isArray(record.api_versions) ||
    !record.api_versions.length ||
    record.api_versions.some((api) => !Number.isSafeInteger(api) || api < 1) ||
    new Set(record.api_versions).size !== record.api_versions.length
  )
    throw new Error("invalid SDK api_versions");
  for (const key of ["host_release", "min_kandev_version"])
    if (!(draft && record[key] === null)) stableParts(record[key]);
  if (
    record.host_release &&
    record.min_kandev_version &&
    compareVersions(record.min_kandev_version, record.host_release) > 0
  )
    throw new Error("SDK minimum exceeds its originating host release");
}
function validateProgression(current, previous) {
  const next = stableParts(current.sdk_version),
    old = stableParts(previous.sdk_version);
  if (compareVersions(current.sdk_version, previous.sdk_version) <= 0)
    throw new Error("SDK version must increase");
  if (current.change_kind === "initial")
    throw new Error("initial change_kind is only valid for the first SDK");
  if (current.change_kind === "breaking") {
    if (old[0] === 0n ? next[0] === 0n && next[1] <= old[1] : next[0] <= old[0])
      throw new Error(
        "breaking SDK changes require a minor bump before 1.0, or a major bump afterward",
      );
  }
  if (
    current.change_kind === "additive" &&
    next[0] === old[0] &&
    next[1] <= old[1]
  )
    throw new Error("additive SDK changes require a minor version bump");
  if (
    current.change_kind === "patch" &&
    (next[0] !== old[0] || next[1] !== old[1])
  )
    throw new Error("patch change_kind requires a patch version bump");
}
export function planSDKRelease(current, previous, changedPaths, hostVersion) {
  validateRecord(current, true);
  if (hostVersion !== null) stableParts(hostVersion);
  if (previous) {
    validateRecord(previous);
    if (previous.sdk_version === current.sdk_version) {
      if (changedPaths.some(isContractInput))
        throw new Error("changed SDK contract requires a new shared version");
      if (!isDeepStrictEqual(previous, current))
        throw new Error("changed SDK release record requires a new version");
      if (
        hostVersion !== null &&
        compareVersions(current.host_release, hostVersion) > 0
      )
        throw new Error("SDK originates after the selected host release");
      return { record: structuredClone(previous), reused: true };
    }
    validateProgression(current, previous);
  } else if (
    current.sdk_version !== "0.1.0" ||
    current.change_kind !== "initial"
  ) {
    throw new Error("first SDK must be version 0.1.0 with initial change_kind");
  }
  if (hostVersion === null)
    return { record: structuredClone(current), reused: false };
  if (current.host_release !== null && current.host_release !== hostVersion)
    throw new Error(
      "new SDK host_release differs from the release being prepared",
    );
  const record = {
    ...current,
    host_release: hostVersion,
    min_kandev_version: current.min_kandev_version ?? hostVersion,
  };
  validateRecord(record);
  return { record, reused: false };
}

const exec = promisify(execFile);
export const recordPath = "pluginsdk/sdk-release.json";
export const npmRecordPath = "apps/packages/plugin-sdk/sdk-release.json";
export async function prepareMetadata({
  root = process.cwd(),
  hostVersion = null,
  stamp = false,
  ref,
} = {}) {
  if (stamp && (ref || hostVersion === null))
    throw new Error("stamping requires a working tree and actual host version");
  if (
    ref &&
    !/^(?:HEAD|[0-9a-f]{40}|(?:refs\/tags\/)?v\d+\.\d+\.\d+)$/.test(ref)
  )
    throw new Error("unsupported host source ref");
  const git = async (...args) =>
    (await exec("git", args, { cwd: root, maxBuffer: 8 * 1024 * 1024 })).stdout;
  const read = (path) =>
    ref ? git("show", `${ref}:${path}`) : readFile(join(root, path), "utf8");
  const raw = await read(recordPath);
  const current = JSON.parse(raw);
  if ((await read(npmRecordPath)) !== raw)
    throw new Error("npm record differs from canonical SDK record");
  const manifest = JSON.parse(
    await read("apps/packages/plugin-sdk/package.json"),
  );
  if (
    manifest.name !== "@kdlbs/plugin-sdk" ||
    manifest.version !== current.sdk_version
  )
    throw new Error("npm package version differs from canonical SDK version");
  const tags = (await git("tag", "--list", "pluginsdk/v*"))
    .trim()
    .split("\n")
    .filter((tag) => stablePattern.test(tag.replace(/^pluginsdk\/v/, "")));
  tags.sort((a, b) =>
    compareVersions(
      a.replace(/^pluginsdk\/v/, ""),
      b.replace(/^pluginsdk\/v/, ""),
    ),
  );
  const previousTag = tags.at(-1);
  const previous = previousTag
    ? JSON.parse(await git("show", `${previousTag}:${recordPath}`))
    : null;
  const paths = previousTag
    ? (
        await git(
          "diff",
          "--name-only",
          "-z",
          previousTag,
          ...(ref ? [ref] : []),
          "--",
        )
      )
        .split("\0")
        .filter(Boolean)
    : [];
  const result = planSDKRelease(current, previous, paths, hostVersion);
  if (stamp && !result.reused) {
    const serialized = JSON.stringify(result.record, null, 2) + "\n";
    await writeFile(join(root, recordPath), serialized);
    await writeFile(join(root, npmRecordPath), serialized);
  }
  return {
    ...result,
    previousTag: previousTag ?? null,
    changedInputs: paths.filter(isContractInput),
  };
}

if (
  process.argv[1] &&
  import.meta.url ===
    (await import("node:url")).pathToFileURL(
      (await import("node:path")).resolve(process.argv[1]),
    ).href
) {
  const { values } = (await import("node:util")).parseArgs({
    options: {
      check: { type: "boolean" },
      preflight: { type: "boolean" },
      stamp: { type: "boolean" },
      "host-version": { type: "string" },
      ref: { type: "string" },
    },
  });
  if (
    [values.check, values.preflight, values.stamp].filter(Boolean).length !== 1
  )
    throw new Error("choose --check, --preflight, or --stamp");
  const result = await prepareMetadata({
    hostVersion: values["host-version"] ?? null,
    stamp: values.stamp,
    ref: values.ref,
  });
  if (values.preflight) {
    if (process.env.SDK_SCOPE_OWNERSHIP_CONFIRMED !== "true")
      throw new Error(
        "maintainer must confirm npm scope/package ownership before release",
      );
    const { readPackage } = await import("./sdk-registry.mjs");
    const registry = await readPackage(result.record.sdk_version);
    result.registry = {
      package_exists: registry.packageExists,
      version_exists: !!registry.version,
    };
  }
  console.log(JSON.stringify(result));
}
