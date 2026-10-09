import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, expect, it, vi } from "vitest";
import { buildE2EImage } from "../fixtures/docker-probe";
import { buildE2ESSHImage } from "../fixtures/ssh-image";

const temporaryDirectories: string[] = [];

afterEach(() => {
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
  for (const directory of temporaryDirectories.splice(0)) {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

it.each([
  ["Docker", buildE2EImage],
  ["SSH", buildE2ESSHImage],
] as const)("preserves %s build stderr and cleans its temporary context", (_name, build) => {
  const existsSync = fs.existsSync;
  const copyFileSync = fs.copyFileSync;
  vi.spyOn(fs, "existsSync").mockImplementation((file) =>
    String(file).endsWith("/backend/bin/mock-agent-linux-amd64") ? true : existsSync(file),
  );
  vi.spyOn(fs, "copyFileSync").mockImplementation((source, destination, mode) => {
    if (String(source).endsWith("/backend/bin/mock-agent-linux-amd64")) {
      fs.writeFileSync(destination, "fixture binary");
      return;
    }
    copyFileSync(source, destination, mode);
  });
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-docker-diagnostic-"));
  temporaryDirectories.push(directory);
  const capture = path.join(directory, "context-path");
  fs.writeFileSync(
    path.join(directory, "docker"),
    '#!/bin/sh\nprintf "%s" "$4" > "$KANDEV_TEST_DOCKER_CAPTURE"\nprintf "fixture registry unavailable\\n" >&2\nexit 23\n',
    { mode: 0o755 },
  );
  vi.stubEnv("PATH", `${directory}${path.delimiter}${process.env.PATH}`);
  vi.stubEnv("KANDEV_TEST_DOCKER_CAPTURE", capture);
  vi.stubEnv("E2E_DEBUG", "");

  expect(() => build()).toThrow("fixture registry unavailable");
  expect(fs.existsSync(fs.readFileSync(capture, "utf8"))).toBe(false);
});
