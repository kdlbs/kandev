import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

const BACKEND_DIR = path.resolve(__dirname, "../../../apps/backend");
const WEB_DIR = path.resolve(__dirname, "..");

export type BackendArtifact = {
  path: string;
  rebuildTarget: string;
};

export default function globalSetup() {
  const customKandevBin = process.env.KANDEV_E2E_BIN;
  const kandevBin = customKandevBin ?? path.join(BACKEND_DIR, "bin", "kandev");
  const artifacts: BackendArtifact[] = [
    { path: path.join(BACKEND_DIR, "bin", "mock-agent"), rebuildTarget: "build" },
    {
      path: path.join(BACKEND_DIR, ".build", "kandev-plugin-e2e-1.0.0.tar.gz"),
      rebuildTarget: "e2e-plugin-package",
    },
  ];

  if (!customKandevBin) {
    artifacts.unshift({ path: kandevBin, rebuildTarget: "build" });
  }
  if (isContainerRun(process.env)) {
    artifacts.push(
      {
        path: path.join(BACKEND_DIR, "bin", "mock-agent-linux-amd64"),
        rebuildTarget: "build-mock-agent-linux",
      },
      {
        path: path.join(BACKEND_DIR, "bin", "agentctl-linux-amd64"),
        rebuildTarget: "build-agentctl-linux",
      },
    );
  }

  const requiredArtifacts = customKandevBin
    ? [{ path: kandevBin, rebuildTarget: undefined }, ...artifacts]
    : artifacts;
  for (const artifact of requiredArtifacts) {
    assertArtifactExists(artifact.path, artifact.rebuildTarget);
  }
  assertBackendArtifactsFresh(BACKEND_DIR, artifacts);

  const spaIndex = path.join(WEB_DIR, "dist", "index.html");
  if (!fs.existsSync(spaIndex)) {
    throw new Error(`Vite web build not found: ${spaIndex}\nRun "make build-web" first.`);
  }

  assertPseudoCatalogBundled();
}

/**
 * Playwright's `globalSetup` receives the fully resolved `FullConfig`, whose
 * `projects` array always lists every project declared in
 * `playwright.config.ts` — it is not filtered down to the project(s) selected
 * via `--project=`. `config.projects.some((p) => p.name === "containers")` is
 * therefore always true and cannot distinguish a `--project=chromium` run
 * from a `--project=containers` one; verified empirically, not just by
 * reading the types.
 *
 * Managed runs and CI pass an explicit environment marker. Direct Playwright
 * runs keep the selected project in `process.argv`, so this function reads the
 * CLI value as well. This mirrors `fixtures/backend.ts`'s
 * `isContainerProjectActive`, whose project-name branch reads
 * `testInfo.project.name` — the per-test resolved project — instead of
 * `FullConfig`.
 */
export function isContainerRun(env: NodeJS.ProcessEnv, argv = process.argv): boolean {
  if (env.KANDEV_E2E_CONTAINERS === "1" || env.KANDEV_E2E_DOCKER === "1") return true;

  const selectedProjects = argv.flatMap((arg, index) => {
    if (arg === "--project") return argv[index + 1] ? [argv[index + 1]!] : [];
    if (arg.startsWith("--project=")) return [arg.slice("--project=".length)];
    return [];
  });

  return selectedProjects.some((projects) =>
    projects
      .split(",")
      .some(
        (project) =>
          project === "containers" || project === "kubernetes-compat" || project === "docker",
      ),
  );
}

function backendMakeCommand(backendDir: string, target: string): string {
  return `make -C ${JSON.stringify(backendDir)} ${target}`;
}

function assertArtifactExists(artifactPath: string, rebuildTarget?: string): void {
  if (fs.existsSync(artifactPath)) return;

  if (!rebuildTarget) {
    throw new Error(
      `The KANDEV_E2E_BIN file does not exist: ${artifactPath}\n` +
        "Set KANDEV_E2E_BIN to an existing backend executable.",
    );
  }
  throw new Error(
    `Required E2E artifact not found: ${artifactPath}\n` +
      `Run "${backendMakeCommand(BACKEND_DIR, rebuildTarget)}" first.`,
  );
}

const BACKEND_SOURCE_SKIP_DIRS = new Set(["bin", ".build", ".git", "testdata"]);

/**
 * Go test-only sources and outputs that never enter production artifacts.
 * `*_test.go` is compiled only by `go test`; `make -C apps/backend
 * test-coverage` writes coverage.out and coverage.html next to go.mod; and
 * `go test -c` leaves `*.test` binaries wherever it is run. None can affect
 * the backend or helper binaries exercised by E2E.
 *
 * Without this, the ordinary `make test-coverage` → run E2E sequence aborts
 * demanding a backend rebuild because a coverage profile — which changes
 * nothing the binary contains — is newer than the binary.
 */
const BACKEND_SOURCE_SKIP_FILE_PATTERNS = [
  /_test\.go$/,
  /^coverage\.out$/,
  /\.test$/,
  /^coverage\.html$/,
];

/**
 * `make sync-embedded-web` copies apps/web/dist here so the binary can serve
 * the SPA itself. E2E never reads that copy — fixtures/backend.ts points the
 * backend at apps/web/dist directly — and refreshing it does not require
 * relinking the binary (`make build-web && make sync-embedded-web` on its own
 * leaves the copy newer than bin/kandev), so counting it would demand a
 * pointless backend rebuild over a bundle the tests ignore.
 */
const EMBEDDED_WEB_DIR = path.join("internal", "webapp", "embedded", "generated");

/**
 * Every file counts, not just `*.go`: the binary `//go:embed`s a large asset
 * surface (`internal/profiles/profiles.yaml`, `config/workflows/*.yml`,
 * `config/prompts/*.md`, `internal/i18n/locales/*.json`,
 * `internal/office/configloader/{instructions,skills}/*`, …). Editing a
 * runtime feature-flag default in profiles.yaml changes backend behaviour
 * exactly as much as editing a `.go` file does, and an extension allowlist
 * cannot tell an embedded `*.md` under config/prompts from a stray AGENTS.md.
 *
 * The bias is deliberate: a spurious hit costs one `make build`, while a miss
 * costs the silent-stale-binary confusion this whole guard exists to prevent.
 */
function findNewestBackendSource(
  backendDir: string,
): { file: string; mtimeMs: number } | undefined {
  let newest: { file: string; mtimeMs: number } | undefined;

  const walk = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const entryPath = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (BACKEND_SOURCE_SKIP_DIRS.has(entry.name)) continue;
        if (path.relative(backendDir, entryPath) === EMBEDDED_WEB_DIR) continue;
        walk(entryPath);
        continue;
      }
      if (!entry.isFile()) continue;
      if (BACKEND_SOURCE_SKIP_FILE_PATTERNS.some((pattern) => pattern.test(entry.name))) continue;

      const mtimeMs = fs.statSync(entryPath).mtimeMs;
      if (!newest || mtimeMs > newest.mtimeMs) {
        newest = { file: entryPath, mtimeMs };
      }
    }
  };

  walk(backendDir);
  return newest;
}

/**
 * `fixtures/backend.ts` spawns the *prebuilt* Go binary — `pnpm run build:e2e`
 * only rebuilds the Vite bundle, so nothing else checks the two agree. A stale
 * binary produces failures that look exactly like product regressions (missing
 * routes return 404, unknown JSON fields are silently dropped) instead of the
 * "rebuild the backend" message a developer actually needs.
 *
 * Compares binary mtime against the newest `apps/backend` source file rather
 * than the embedded git sha, since the sha still matches HEAD with uncommitted
 * backend edits — the more common case during local development.
 */
export function assertBackendArtifactsFresh(
  backendDir: string,
  artifacts: BackendArtifact[],
  env: NodeJS.ProcessEnv = process.env,
): void {
  if (env.KANDEV_E2E_SKIP_FRESHNESS === "1") return;
  if (validateE2EBuildIdentity(backendDir, artifacts, env)) return;

  const newestSource = findNewestBackendSource(backendDir);
  if (!newestSource) return;

  for (const artifact of artifacts) {
    const artifactMtimeMs = fs.statSync(artifact.path).mtimeMs;
    if (artifactMtimeMs < newestSource.mtimeMs) {
      throw new Error(
        `Required E2E artifact is older than apps/backend sources: ${artifact.path}. ` +
          `${newestSource.file} is newer. Run "${backendMakeCommand(
            backendDir,
            artifact.rebuildTarget,
          )}". E2E uses prebuilt backend artifacts. The frontend build does not rebuild them.`,
      );
    }
  }
}

type E2EBuildIdentity = {
  schema_version: number;
  source_revision: string;
  artifacts: Record<string, string>;
};

function sha256File(filePath: string): string {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function readE2EBuildIdentity(identityPath: string): E2EBuildIdentity {
  let identity: E2EBuildIdentity;
  try {
    identity = JSON.parse(fs.readFileSync(path.resolve(identityPath), "utf8")) as E2EBuildIdentity;
  } catch (error) {
    throw new Error(`Cannot read E2E build identity ${identityPath}`, { cause: error });
  }
  if (identity.schema_version !== 1 || !identity.artifacts) {
    throw new Error(`Unsupported E2E build identity schema in ${identityPath}`);
  }
  return identity;
}

function validateE2EBuildIdentity(
  backendDir: string,
  artifacts: BackendArtifact[],
  env: NodeJS.ProcessEnv,
): boolean {
  const identityPath = env.KANDEV_E2E_BUILD_IDENTITY?.trim();
  const expectedRevision = env.KANDEV_E2E_EXPECTED_SOURCE_REVISION?.trim();
  if (!identityPath && !expectedRevision) return false;
  if (!identityPath || !expectedRevision) {
    throw new Error(
      "KANDEV_E2E_BUILD_IDENTITY and KANDEV_E2E_EXPECTED_SOURCE_REVISION must be set together",
    );
  }

  const identity = readE2EBuildIdentity(identityPath);
  if (identity.source_revision !== expectedRevision) {
    throw new Error(
      `E2E build identity source revision ${identity.source_revision} does not match checkout ${expectedRevision}`,
    );
  }

  for (const artifact of artifacts) {
    const key = path.relative(backendDir, artifact.path).split(path.sep).join("/");
    if (key.startsWith("../") || path.isAbsolute(key)) {
      throw new Error(`E2E artifact is outside the backend directory: ${artifact.path}`);
    }
    const expectedChecksum = identity.artifacts[key];
    if (!expectedChecksum) {
      throw new Error(`E2E build identity does not cover ${key}`);
    }
    const actualChecksum = sha256File(artifact.path);
    if (actualChecksum !== expectedChecksum) {
      throw new Error(
        `E2E build identity checksum mismatch for ${key}: got ${actualChecksum}, expected ${expectedChecksum}`,
      );
    }
  }
  return true;
}

/**
 * Fail fast when the coverage oracle is asked to run against a bundle that has
 * no pseudo catalog.
 *
 * A production `vite build` deliberately drops it (see lib/i18n/bundling.ts).
 * Run tests/i18n/pseudo-coverage.spec.ts against such a bundle and every screen
 * falls back to `en` — so all ten tests fail with a wall of findings that reads
 * exactly like a mass externalization regression, when the real cause is that
 * the artifact was built with `make build-web` instead of `make build-web-e2e`.
 * Only checked under KANDEV_I18N_COVERAGE=1; nothing else needs the catalog.
 *
 * The markers are read from the catalogs themselves rather than hardcoded, so
 * this cannot drift as `pnpm run i18n:pseudo` regenerates them — and the whole
 * SOURCE directory is scanned rather than one named file, so adding, renaming or
 * splitting a namespace cannot quietly turn the check into a no-op. Every path
 * out of here either throws or has actually compared markers against `dist/`: a
 * guard that can only ever pass is worth less than no guard, because it also
 * reports clean.
 */
function assertPseudoCatalogBundled() {
  if (process.env.KANDEV_I18N_COVERAGE !== "1") return;

  const catalogDir = path.join(WEB_DIR, "src", "locales", "pseudo");
  const catalogs = fs.existsSync(catalogDir)
    ? fs.readdirSync(catalogDir).filter((name) => name.endsWith(".json"))
    : [];
  const markers = catalogs
    .flatMap((name) =>
      Object.values(
        JSON.parse(fs.readFileSync(path.join(catalogDir, name), "utf8")) as Record<string, unknown>,
      ),
    )
    .filter((value): value is string => typeof value === "string" && value.length >= 20)
    .slice(0, 50);

  if (markers.length === 0) {
    throw new Error(
      `KANDEV_I18N_COVERAGE=1 but no usable pseudo source catalog was found in ${catalogDir}.\n` +
        'The coverage oracle cannot mean anything without it. Run "pnpm run i18n:pseudo" ' +
        "to regenerate it. See docs/i18n.md.",
    );
  }

  const assets = path.join(WEB_DIR, "dist", "assets");
  const bundled = fs
    .readdirSync(assets)
    .filter((name) => name.endsWith(".js"))
    .some((name) => {
      const source = fs.readFileSync(path.join(assets, name), "utf8");
      return markers.some((marker) => source.includes(marker));
    });

  if (!bundled) {
    throw new Error(
      "KANDEV_I18N_COVERAGE=1 but dist/ contains no pseudo catalog, so the coverage " +
        "oracle would report every screen as un-externalized.\n" +
        'Rebuild the SPA with the QA locale: "make build-web-e2e" ' +
        '(or "pnpm --filter @kandev/web build:e2e"). See docs/i18n.md.',
    );
  }
}
