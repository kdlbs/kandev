import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { buildE2EImage } from "../fixtures/docker-probe";

const temporaryDirectories: string[] = [];

afterEach(() => {
  vi.unstubAllEnvs();
  for (const directory of temporaryDirectories.splice(0)) {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

function installFakeDocker(script: string): void {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-fake-docker-"));
  temporaryDirectories.push(directory);
  const executable = path.join(directory, "docker");
  fs.writeFileSync(executable, `#!/usr/bin/env node\n${script}\n`, { mode: 0o755 });
  vi.stubEnv("PATH", `${directory}${path.delimiter}${process.env.PATH ?? ""}`);
  vi.stubEnv("E2E_DEBUG", "");
}

describe("Docker fixture command diagnostics", () => {
  it("includes bounded child output when an image build fails", () => {
    installFakeDocker(
      [
        'process.stdout.write("o".repeat(10_000) + "stdout-tail-sentinel");',
        'process.stderr.write("e".repeat(10_000) + "stderr-tail-sentinel");',
        "process.exit(17);",
      ].join("\n"),
    );

    let failure: unknown;
    try {
      buildE2EImage();
    } catch (error) {
      failure = error;
    }

    expect(failure).toBeInstanceOf(Error);
    const message = (failure as Error).message;
    expect(message).toContain("stdout-tail-sentinel");
    expect(message).toContain("stderr-tail-sentinel");
    expect(message).toContain("17");
    expect(message).toContain("truncated");
    expect(message.length).toBeLessThan(10_000);
  });

  it("keeps a successful image build non-throwing", () => {
    installFakeDocker(
      [
        'process.stdout.write("successful build stdout");',
        'process.stderr.write("successful build stderr");',
      ].join("\n"),
    );

    expect(buildE2EImage()).toBeUndefined();
  });

  it("does not fail successful builds that emit more than 1 MiB", () => {
    installFakeDocker(
      [
        'process.stdout.write("o".repeat(1_100_000));',
        'process.stderr.write("e".repeat(1_100_000));',
      ].join("\n"),
    );

    expect(buildE2EImage()).toBeUndefined();
  });
});
