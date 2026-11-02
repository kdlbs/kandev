import { execFile } from "node:child_process";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { promisify, parseArgs } from "node:util";
import { integrityOf, inspectArchive } from "../../../../scripts/pluginsdk/pack-candidate.mjs";
import { startRegistry, npmEnvironment } from "../../../../scripts/pluginsdk/npm-consumer.mjs";

const run = promisify(execFile);
export async function checkPackedConsumer(
  candidate,
  inspectConsumer,
  { mode = "local", config } = {},
) {
  if ((await integrityOf(candidate.archive)) !== candidate.integrity)
    throw new Error("candidate integrity mismatch before installation");
  await inspectArchive(candidate.archive, config);
  const root = await mkdtemp(join(tmpdir(), "sdk-packed-consumer-"));
  let registry;
  try {
    if (mode === "public") {
      const { validatePublicTrust } =
        await import("../../../../scripts/pluginsdk/public-trust.mjs");
      await validatePublicTrust();
    } else registry = await startRegistry(candidate);
    const registryURL = registry?.url ?? "https://registry.npmjs.org";
    const cwd = join(root, "consumer");
    await mkdir(cwd);
    await writeFile(
      join(cwd, "package.json"),
      JSON.stringify({
        name: "independent-sdk-consumer",
        private: true,
        type: "module",
        dependencies: { "@kdlbs/plugin-sdk": candidate.sdk_version },
        devDependencies: { typescript: "5.9.3" },
      }),
    );
    const env = npmEnvironment(root);
    await run("npm", ["install", "--registry", registryURL], { cwd, env, timeout: 180_000 });
    const lock = JSON.parse(await readFile(join(cwd, "package-lock.json"), "utf8"));
    const installed = lock.packages["node_modules/@kdlbs/plugin-sdk"];
    if (
      installed.integrity !== candidate.integrity ||
      !installed.resolved.startsWith(registryURL + "/")
    )
      throw new Error("consumer did not install the candidate from the selected registry");
    await writeFile(
      join(cwd, "consumer.ts"),
      `import type { PluginHostApi, PluginRegistry, PluginComposerSlotProps } from '@kdlbs/plugin-sdk';\nimport record from '@kdlbs/plugin-sdk/sdk-release.json' with { type: 'json' };\ndeclare const host: PluginHostApi;\ndeclare const registry: PluginRegistry;\ndeclare const slot: PluginComposerSlotProps;\nvoid host.React; void registry; void slot; const version: string = record.sdk_version; void version;\n`,
    );
    const modes = ["NodeNext", "Bundler"];
    for (const mode of modes) {
      await writeFile(
        join(cwd, "tsconfig.json"),
        JSON.stringify({
          compilerOptions: {
            target: "ES2022",
            module: mode === "Bundler" ? "ESNext" : "NodeNext",
            moduleResolution: mode,
            strict: true,
            noEmit: true,
            resolveJsonModule: true,
            skipLibCheck: false,
          },
          include: ["consumer.ts"],
        }),
      );
      await run(
        process.execPath,
        [join(cwd, "node_modules/typescript/bin/tsc"), "--project", "tsconfig.json"],
        { cwd, env },
      );
    }
    if (mode === "public")
      await run("npm", ["audit", "signatures", "--registry", registryURL], {
        cwd,
        env,
        timeout: 180_000,
      });
    await inspectConsumer?.({ cwd, env, root });
    if ((await integrityOf(candidate.archive)) !== candidate.integrity)
      throw new Error("candidate integrity mismatch after installation");
    return { modes, integrity: candidate.integrity, version: installed.version };
  } finally {
    await registry?.close();
    await rm(root, { recursive: true, force: true });
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const { values } = parseArgs({ options: { candidate: { type: "string" } } });
  if (values.candidate)
    console.log(
      JSON.stringify(
        await checkPackedConsumer(JSON.parse(await readFile(values.candidate, "utf8"))),
      ),
    );
}
