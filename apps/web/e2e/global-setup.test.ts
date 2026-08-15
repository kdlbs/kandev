import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { assertBackendBinaryFresh } from "./global-setup";

let backendDir: string;
let binPath: string;

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
  fs.rmSync(backendDir, { recursive: true, force: true });
});

function touch(filePath: string, mtime: Date) {
  fs.utimesSync(filePath, mtime, mtime);
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
      /Backend binary is older than apps\/backend sources.*Run "make -C apps\/backend build"/s,
    );
  });

  it("skips the check when KANDEV_E2E_BIN is set", () => {
    touch(binPath, new Date("2026-01-01T00:00:00Z"));
    touch(
      path.join(backendDir, "internal", "task", "service.go"),
      new Date("2026-01-02T00:00:00Z"),
    );

    expect(() =>
      assertBackendBinaryFresh(backendDir, [binPath], { KANDEV_E2E_BIN: "/some/other/kandev" }),
    ).not.toThrow();
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
