import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import globalSetup, { assertBackendArtifactsFresh, isContainerRun } from "./global-setup";

let backendDir: string;
let binPath: string;
const originalArgv = [...process.argv];

beforeEach(() => {
  backendDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-backend-fixture-"));
  fs.mkdirSync(path.join(backendDir, "bin"), { recursive: true });
  fs.mkdirSync(path.join(backendDir, "internal", "task"), { recursive: true });
  fs.writeFileSync(path.join(backendDir, "internal", "task", "service.go"), "package task\n");
  fs.writeFileSync(path.join(backendDir, "go.mod"), "module kandev\n");
  fs.utimesSync(
    path.join(backendDir, "go.mod"),
    new Date("2020-01-01T00:00:00Z"),
    new Date("2020-01-01T00:00:00Z"),
  );
  binPath = path.join(backendDir, "bin", "kandev");
  fs.writeFileSync(binPath, "binary");
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
  process.argv = [...originalArgv];
  fs.rmSync(backendDir, { recursive: true, force: true });
});

function touch(filePath: string, mtime: Date) {
  fs.utimesSync(filePath, mtime, mtime);
}

function assertBackendBinaryFresh(
  fixtureBackendDir: string,
  binPaths: string[],
  env: NodeJS.ProcessEnv,
): void {
  assertBackendArtifactsFresh(
    fixtureBackendDir,
    binPaths.map((artifactPath) => ({ path: artifactPath, rebuildTarget: "build" })),
    env,
  );
}

describe("assertBackendBinaryFresh", () => {
  it("passes when the binary is newer than every backend source file", () => {
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2026-01-01T00:00:00Z"),
    );
    touch(binPath, new Date("2026-01-02T00:00:00Z"));

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).not.toThrow();
  });

  it("throws with the remedy when a .go source file is newer than the binary", () => {
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    const staleSource = path.join(backendDir, "internal", "task", "service.go");
    touch(staleSource, new Date("2026-01-02T00:00:00Z"));

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).toThrow(
      /Required E2E artifact is older than apps\/backend sources.*service\.go/s,
    );
  });

  it("still checks the local mock agent when KANDEV_E2E_BIN selects a custom backend", () => {
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2026-01-02T00:00:00Z"),
    );

    expect(() =>
      assertBackendBinaryFresh(backendDir, [binPath], { KANDEV_E2E_BIN: "/some/other/kandev" }),
    ).toThrow(/service\.go/);
  });

  it("skips the check when KANDEV_E2E_SKIP_FRESHNESS=1", () => {
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2026-01-02T00:00:00Z"),
    );

    expect(() =>
      assertBackendBinaryFresh(backendDir, [binPath], { KANDEV_E2E_SKIP_FRESHNESS: "1" }),
    ).not.toThrow();
  });

  it("throws when an embedded non-Go asset is newer than the binary", () => {
    // profiles.yaml is //go:embed-ed, so a runtime feature-flag edit here
    // changes backend behaviour without touching a single .go file.
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2025-01-01T00:00:00Z"),
    );
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    fs.mkdirSync(path.join(backendDir, "internal", "profiles"), { recursive: true });
    const profiles = path.join(backendDir, "internal", "profiles", "profiles.yaml");
    fs.writeFileSync(profiles, "e2e:\n");
    touch(profiles, new Date("2026-01-02T00:00:00Z"));

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).toThrow(/profiles\.yaml/);
  });

  it("ignores the synced web bundle under internal/webapp/embedded/generated", () => {
    // A copy of apps/web/dist that E2E never reads — fixtures/backend.ts serves
    // apps/web/dist directly — and `make sync-embedded-web` refreshes it without
    // relinking the binary, so counting it would demand a pointless rebuild.
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2025-01-01T00:00:00Z"),
    );
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    const generated = path.join(backendDir, "internal", "webapp", "embedded", "generated");
    fs.mkdirSync(generated, { recursive: true });
    fs.writeFileSync(path.join(generated, "index.html"), "<html></html>");
    touch(path.join(generated, "index.html"), new Date("2026-06-01T00:00:00Z"));

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).not.toThrow();
  });

  it("ignores Go coverage and compiled-test artifacts left in the source tree", () => {
    // `make -C apps/backend test-coverage` writes these next to go.mod. They are
    // gitignored build outputs, so a test run must not demand a backend rebuild.
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2025-01-01T00:00:00Z"),
    );
    touch(binPath, new Date("2026-01-01T00:00:00Z"));

    for (const name of ["coverage.out", "coverage.html", "task.test"]) {
      const artifact = path.join(backendDir, name);
      fs.writeFileSync(artifact, "generated");
      touch(artifact, new Date("2026-06-01T00:00:00Z"));
    }

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).not.toThrow();
  });

  it("still reports a stale binary when a coverage run sits alongside a real source edit", () => {
    // The property that makes the artifact exclusion safe: it removes those files
    // from consideration, it does not short-circuit the check. Widening the skip
    // patterns far enough to swallow real sources has to fail here.
    touch(binPath, new Date("2026-01-01T00:00:00Z"));

    const artifact = path.join(backendDir, "coverage.out");
    fs.writeFileSync(artifact, "mode: set\n");
    touch(artifact, new Date("2026-06-01T00:00:00Z"));

    const edited = path.join(backendDir, "internal", "task", "service.go");
    touch(edited, new Date("2026-06-02T00:00:00Z"));

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).toThrow(/service\.go/);
  });

  it("only skips exact artifact names, not files that merely resemble them", () => {
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2025-01-01T00:00:00Z"),
    );

    const lookalike = path.join(backendDir, "internal", "task", "notes.out");
    fs.writeFileSync(lookalike, "not a coverage profile\n");
    touch(lookalike, new Date("2026-06-01T00:00:00Z"));

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).toThrow(/notes\.out/);
  });

  it("checks every required binary when only the second binary is stale", () => {
    const mockAgentPath = path.join(backendDir, "bin", "mock-agent");
    fs.writeFileSync(mockAgentPath, "binary");
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2026-01-02T00:00:00Z"),
    );
    touch(binPath, new Date("2026-01-03T00:00:00Z"));
    touch(mockAgentPath, new Date("2026-01-01T00:00:00Z"));

    expect(() => assertBackendBinaryFresh(backendDir, [binPath, mockAgentPath], {})).toThrow(
      /mock-agent/,
    );
  });

  it("prints a rebuild command that works from the current directory", () => {
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2026-01-02T00:00:00Z"),
    );

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).toThrow(
      `make -C ${JSON.stringify(backendDir)} build`,
    );
  });

  it("uses the fixture package rebuild target when that package is stale", () => {
    const pluginPackage = path.join(backendDir, ".build", "kandev-plugin-e2e-1.0.0.tar.gz");
    fs.mkdirSync(path.dirname(pluginPackage), { recursive: true });
    fs.writeFileSync(pluginPackage, "package");
    touch(pluginPackage, new Date("2026-01-01T00:00:00Z"));
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2026-01-02T00:00:00Z"),
    );

    expect(() =>
      assertBackendArtifactsFresh(
        backendDir,
        [{ path: pluginPackage, rebuildTarget: "e2e-plugin-package" }],
        {},
      ),
    ).toThrow(`make -C ${JSON.stringify(backendDir)} e2e-plugin-package`);
  });

  it("ignores files under bin/, .build/, and testdata/ when finding the newest source", () => {
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2025-12-01T00:00:00Z"),
    );

    fs.mkdirSync(path.join(backendDir, ".build"), { recursive: true });
    fs.writeFileSync(path.join(backendDir, ".build", "artifact.go"), "package build\n");
    touch(path.join(backendDir, ".build", "artifact.go"), new Date("2026-06-01T00:00:00Z"));

    fs.mkdirSync(path.join(backendDir, "internal", "task", "testdata"), { recursive: true });
    fs.writeFileSync(
      path.join(backendDir, "internal", "task", "testdata", "fixture.go"),
      "package task\n",
    );
    touch(
      path.join(backendDir, "internal", "task", "testdata", "fixture.go"),
      new Date("2026-06-01T00:00:00Z"),
    );

    expect(() => assertBackendBinaryFresh(backendDir, [binPath], {})).not.toThrow();
  });
});

describe("globalSetup", () => {
  it.each([
    ["containers", ["node", "playwright", "test", "--project=containers"]],
    ["docker", ["node", "playwright", "test", "--project=docker"]],
    ["containers", ["node", "playwright", "test", "--project", "containers"]],
    ["docker", ["node", "playwright", "test", "--project", "docker"]],
  ])("recognizes the direct %s project selection", (_project, argv) => {
    expect(isContainerRun({}, argv)).toBe(true);
  });

  it("checks the selected custom backend path instead of the default backend", () => {
    const customBackend = path.join(backendDir, "custom", "kandev");
    vi.stubEnv("KANDEV_E2E_BIN", customBackend);
    vi.stubEnv("KANDEV_E2E_SKIP_FRESHNESS", "1");
    const existsSync = vi.spyOn(fs, "existsSync").mockReturnValue(true);

    (globalSetup as () => void)();

    expect(existsSync).toHaveBeenCalledWith(customBackend);
    expect(existsSync).not.toHaveBeenCalledWith(path.join(backendDirForTest(), "bin", "kandev"));
  });

  it("reports a missing custom backend path", () => {
    const customBackend = path.join(backendDir, "missing", "kandev");
    vi.stubEnv("KANDEV_E2E_BIN", customBackend);
    const existsSync = vi
      .spyOn(fs, "existsSync")
      .mockImplementation((artifactPath) => artifactPath !== customBackend);

    expect(() => (globalSetup as () => void)()).toThrow(/KANDEV_E2E_BIN file does not exist/);
    expect(existsSync).toHaveBeenCalledWith(customBackend);
  });

  // Regression: `globalSetup` used to derive "is this a container run" from
  // Playwright's `FullConfig.projects`, which always lists every project
  // declared in playwright.config.ts regardless of `--project=` selection —
  // so an ordinary `--project=chromium` run was indistinguishable from
  // `--project=containers` and wrongly demanded Linux-only helper binaries
  // that `make build` does not produce. See global-setup.ts's isContainerRun.
  it.each([
    ["KANDEV_E2E_CONTAINERS", "1"],
    ["KANDEV_E2E_DOCKER", "1"],
  ])("checks Linux helper binaries for the %s marker", (marker, value) => {
    vi.stubEnv("KANDEV_E2E_SKIP_FRESHNESS", "1");
    vi.stubEnv(marker, value);
    const existsSync = vi.spyOn(fs, "existsSync").mockReturnValue(true);

    (globalSetup as () => void)();

    expect(existsSync).toHaveBeenCalledWith(
      path.join(backendDirForTest(), "bin", "mock-agent-linux-amd64"),
    );
    expect(existsSync).toHaveBeenCalledWith(
      path.join(backendDirForTest(), "bin", "agentctl-linux-amd64"),
    );
  });

  it("checks Linux helper binaries for a direct containers project selection", () => {
    vi.stubEnv("KANDEV_E2E_SKIP_FRESHNESS", "1");
    vi.stubEnv("KANDEV_E2E_CONTAINERS", "");
    vi.stubEnv("KANDEV_E2E_DOCKER", "");
    process.argv = [...originalArgv, "--project=containers"];
    const existsSync = vi.spyOn(fs, "existsSync").mockReturnValue(true);

    (globalSetup as () => void)();

    expect(existsSync).toHaveBeenCalledWith(
      path.join(backendDirForTest(), "bin", "mock-agent-linux-amd64"),
    );
    expect(existsSync).toHaveBeenCalledWith(
      path.join(backendDirForTest(), "bin", "agentctl-linux-amd64"),
    );
  });

  it("does not require Linux helper binaries for an ordinary run", () => {
    vi.stubEnv("KANDEV_E2E_SKIP_FRESHNESS", "1");
    vi.stubEnv("KANDEV_E2E_CONTAINERS", "");
    vi.stubEnv("KANDEV_E2E_DOCKER", "");
    const existsSync = vi.spyOn(fs, "existsSync").mockReturnValue(true);

    (globalSetup as () => void)();

    expect(existsSync).not.toHaveBeenCalledWith(
      path.join(backendDirForTest(), "bin", "mock-agent-linux-amd64"),
    );
    expect(existsSync).not.toHaveBeenCalledWith(
      path.join(backendDirForTest(), "bin", "agentctl-linux-amd64"),
    );
  });
});

function backendDirForTest(): string {
  return path.resolve(__dirname, "../../../apps/backend");
}
