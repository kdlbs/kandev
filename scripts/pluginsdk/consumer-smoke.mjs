import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, mkdir, writeFile, readFile, rm, cp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { parseArgs } from "node:util";
import { sdkModule, goEnvironment, startProxy } from "./go-consumer.mjs";

const run = promisify(execFile);
const repository = fileURLToPath(new URL("../../", import.meta.url));

export async function archiveModule(source, proxyRoot, version) {
  if (!/^v\d+\.\d+\.\d+$/.test(version))
    throw new Error("expected an exact SDK version");
  const output = join(proxyRoot, sdkModule, "@v");
  await mkdir(output, { recursive: true });
  const goMod = await readFile(join(source, "go.mod"), "utf8");
  if (
    !goMod.startsWith(`module ${sdkModule}\n`) ||
    /^\s*replace(?:\s|\()/m.test(goMod)
  ) {
    throw new Error("SDK must declare its public module without replacements");
  }
  await writeFile(join(output, `${version}.mod`), goMod);
  await writeFile(
    join(output, `${version}.info`),
    JSON.stringify({ Version: version, Time: "2026-01-01T00:00:00Z" }),
  );
  await writeFile(join(output, "list"), version + "\n");
  await run("python3", [
    "-c",
    `
import pathlib, sys, zipfile
source, output, prefix = pathlib.Path(sys.argv[1]), sys.argv[2], sys.argv[3]
with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED) as archive:
 for path in sorted(source.rglob('*')):
  rel = path.relative_to(source)
  if any(part.startswith('.') or part == 'node_modules' for part in rel.parts): continue
  if path.is_symlink(): raise ValueError('module archive contains a symlink: ' + str(rel))
  if path.is_file(): archive.write(path, prefix + '/' + rel.as_posix())
`,
    source,
    join(output, `${version}.zip`),
    `${sdkModule}@${version}`,
  ]);
}

async function exerciseGo(cwd, env, version) {
  await writeFile(
    join(cwd, "go.mod"),
    `module example.com/sdk-consumer\n\ngo 1.26.0\n\nrequire ${sdkModule} ${version}\n`,
  );
  await writeFile(
    join(cwd, "main.go"),
    `package main\nimport "${sdkModule}"\nfunc main() { pluginsdk.Serve(&pluginsdk.UnimplementedPlugin{}) }\n`,
  );
  await writeFile(
    join(cwd, "main_test.go"),
    `package main\nimport ("testing"; "${sdkModule}")\nfunc TestPluginContract(t *testing.T) { var plugin pluginsdk.Plugin = &pluginsdk.UnimplementedPlugin{}; if plugin == nil { t.Fatal("missing plugin") } }\n`,
  );
  const go = (args) =>
    run("go", args, {
      cwd,
      env,
      maxBuffer: 8 * 1024 * 1024,
      timeout: 300_000,
    });
  await go(["mod", "tidy"]);
  const { stdout: graph } = await go(["list", "-m", "all"]);
  if (
    graph
      .split("\n")
      .some((line) => line.startsWith("github.com/kandev/kandev "))
  ) {
    throw new Error("consumer graph includes backend module");
  }
  const { stdout: selected } = await go(["list", "-m", "-json", sdkModule]);
  const module = JSON.parse(selected);
  if (module.Replace || module.Version !== version)
    throw new Error("consumer selected an unexpected SDK");
  for (const args of [
    ["build", "-trimpath", "./..."],
    ["vet", "./..."],
    ["test", "-trimpath", "./..."],
  ])
    await go(args);
  return { go, graph, module };
}

async function localSources(root, version, resolved, sourceRoot) {
  const sdk = join(root, "source", "pluginsdk");
  const frontend = join(root, "source", "frontend");
  const filter = (path) =>
    !path.split("/").some((part) => ["node_modules", ".tools"].includes(part));
  await cp(join(sourceRoot, "pluginsdk"), sdk, { recursive: true, filter });
  await cp(join(sourceRoot, "apps/packages/plugin-sdk"), frontend, {
    recursive: true,
    filter,
  });
  const record = JSON.parse(
    await readFile(join(sdk, "sdk-release.json"), "utf8"),
  );
  if (record.sdk_version !== version.replace(/^v/, ""))
    throw new Error("local archive version differs from source record");
  if (
    resolved &&
    (record.host_release === null || record.min_kandev_version === null)
  ) {
    record.host_release ??= "0.100.0";
    record.min_kandev_version ??= record.host_release;
    for (const directory of [sdk, frontend])
      await writeFile(
        join(directory, "sdk-release.json"),
        JSON.stringify(record, null, 2) + "\n",
      );
  }
  return { sdk, frontend, record };
}

async function checkConsumers(go, cwd, version, record, frontend) {
  const command = ["run", `${sdkModule}/cmd/kandev-sdk-check@${version}`];
  const args = frontend
    ? ["--frontend-sdk", "node_modules/@kdlbs/plugin-sdk"]
    : [];
  const manifest = join(cwd, "manifest.yaml");
  const valid = `api_version: ${record.api_versions.at(-1)}\nmin_kandev_version: ${record.min_kandev_version}\n`;
  await writeFile(manifest, valid);
  const paths = [[], ...(frontend ? [args, [...args, "--frontend-only"]] : [])];
  for (const flags of paths) await go([...command, ...flags]);
  const before = await readFile(join(cwd, "go.mod"), "utf8");
  await writeFile(manifest, "api_version: 2\nmin_kandev_version: 0.0.0\n");
  try {
    await go([...command, ...args]);
    throw new Error("checker accepted an insufficient host minimum");
  } catch (error) {
    if (!error.stderr?.includes("requires min_kandev_version")) throw error;
  }
  await writeFile(manifest, valid);
  if ((await readFile(join(cwd, "go.mod"), "utf8")) !== before)
    throw new Error("checker modified the consumer go.mod");
}

export async function smokeGo({
  mode = "local",
  version = "v0.1.0",
  skipCompatibilityCheck = false,
  frontend = false,
  sourceRoot = repository,
  candidate: suppliedCandidate,
  expectedRecordDigest,
  packConfig,
} = {}) {
  if (mode === "public") {
    if (!suppliedCandidate || !expectedRecordDigest || skipCompatibilityCheck)
      throw new Error(
        "public verification requires an authenticated candidate and record",
      );
    const { validatePublicTrust } = await import("./public-trust.mjs");
    await validatePublicTrust();
  }
  const root = await mkdtemp(join(tmpdir(), "kandev-sdk-consumer-"));
  let proxy;
  try {
    let source;
    if (mode === "local") {
      source = await localSources(
        root,
        version,
        !skipCompatibilityCheck,
        sourceRoot,
      );
      await archiveModule(source.sdk, join(root, "proxy"), version);
      proxy = await startProxy(join(root, "proxy"));
    }
    const env = goEnvironment(
      mode,
      root,
      proxy?.url,
      mode === "public"
        ? { ...process.env, GOPROXY: "https://proxy.golang.org" }
        : process.env,
    );
    const exercise = async (cwd) => {
      const { go, graph, module } = await exerciseGo(cwd, env, version);
      if (expectedRecordDigest) {
        const { digest } = await import("../release/sdk-artifact.mjs");
        if (
          digest(await readFile(join(module.Dir, "sdk-release.json"))) !==
          expectedRecordDigest
        )
          throw new Error("public Go record differs from authenticated source");
      }
      if (!skipCompatibilityCheck) {
        const record = JSON.parse(
          await readFile(join(module.Dir, "sdk-release.json"), "utf8"),
        );
        await checkConsumers(go, cwd, version, record, frontend);
      }
      console.log(
        JSON.stringify({
          mode,
          version,
          module: sdkModule,
          cache: root,
          graph,
          compatibility: skipCompatibilityCheck ? "skipped" : "passed",
          status: "passed",
        }),
      );
    };
    if (frontend) {
      const { packCandidate } = await import("./pack-candidate.mjs");
      const { checkPackedConsumer } =
        await import("../../apps/packages/plugin-sdk/test/packed-consumer.node.mjs");
      const candidate =
        suppliedCandidate ??
        (await packCandidate({
          directory: source.frontend,
          output: join(root, "candidate"),
          config: packConfig,
        }));
      console.log(
        JSON.stringify(
          await checkPackedConsumer(candidate, ({ cwd }) => exercise(cwd), {
            mode,
            config: packConfig,
          }),
        ),
      );
    } else {
      const cwd = join(root, "consumer");
      await mkdir(cwd);
      await exercise(cwd);
    }
  } finally {
    await proxy?.close();
    await rm(root, { recursive: true, force: true });
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  const { values } = parseArgs({
    options: {
      mode: { type: "string", default: "local" },
      version: { type: "string", default: "v0.1.0" },
      "host-tag": { type: "string" },
      "backend-only": { type: "boolean" },
      "skip-compatibility-check": { type: "boolean" },
    },
  });
  if (values.mode === "public") {
    const { runRelease } = await import("../release/publish-plugin-sdk.mjs");
    await runRelease({ mode: "verify", hostTag: values["host-tag"] });
  } else
    await smokeGo({
      frontend: !values["backend-only"],
      mode: values.mode,
      version: values.version,
      skipCompatibilityCheck: values["skip-compatibility-check"],
    });
}
