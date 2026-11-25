import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { performance } from "node:perf_hooks";
import type { TestInfo } from "@playwright/test";
import { afterEach, describe, expect, it, vi } from "vitest";
import { captureSSHInterruptionLogs } from "./ssh-failure-diagnostics";
import { execInContainer, type SSHServerHandle } from "./ssh";

const roots: string[] = [];
const credential = "FAKE-CREDENTIAL-DO-NOT-RETAIN";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
  for (const root of roots.splice(0)) fs.rmSync(root, { recursive: true, force: true });
});

function fakeDocker(root: string, script: string): SSHServerHandle {
  fs.writeFileSync(path.join(root, "docker"), `#!${process.execPath}\n${script}\n`, {
    mode: 0o755,
  });
  vi.stubEnv("PATH", `${root}${path.delimiter}${process.env.PATH ?? ""}`);
  return {
    containerId: "diagnostic-container",
    containerName: "diagnostic-container",
    host: "127.0.0.1",
    port: 22,
    user: "kandev",
    identityFile: path.join(root, "id_ed25519"),
    publicKeyFile: path.join(root, "id_ed25519.pub"),
    hostFingerprint: "SHA256:diagnostic-fixture",
    workDir: root,
  };
}

function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-ssh-diagnostics-"));
  roots.push(root);
  const output = path.join(root, "output");
  const copies = path.join(output, "attachments");
  fs.mkdirSync(copies, { recursive: true });
  const backend = { logPath: path.join(root, "structured.log"), tmpDir: root };
  const remote = path.join(root, "agentctl.log");
  const sources = [backend.logPath, path.join(root, "backend-process.log"), remote];
  for (const source of sources) fs.writeFileSync(source, "retained recovery event\n");
  const attach = vi.fn(async (name: string, options: { path: string }) => {
    fs.copyFileSync(options.path, path.join(copies, name));
  });
  const outputPath = vi.fn((name: string) => path.join(output, name));
  const info = {
    status: "failed",
    expectedStatus: "passed",
    attach,
    outputPath,
  } as unknown as TestInfo;
  const metadata = () =>
    JSON.parse(fs.readFileSync(path.join(output, "ssh-diagnostics.json"), "utf8"));
  return { root, output, copies, backend, remote, sources, info, attach, outputPath, metadata };
}

describe("captureSSHInterruptionLogs", () => {
  it.each([
    ["passed", "passed"],
    ["failed", "failed"],
    ["passed", "failed"],
    ["skipped", "passed"],
  ] as const)("does no I/O for status %s expected %s", async (status, expectedStatus) => {
    const state = fixture();
    const open = vi.spyOn(fs, "openSync");
    const write = vi.spyOn(fs, "writeFileSync");
    await captureSSHInterruptionLogs(
      { ...state.info, status, expectedStatus },
      state.backend,
      state.remote,
    );
    expect(open).not.toHaveBeenCalled();
    expect(write).not.toHaveBeenCalled();
    expect(state.outputPath).not.toHaveBeenCalled();
    expect(state.attach).not.toHaveBeenCalled();
  });

  it("bounds aggregate retained bytes including attachment copies and records truncated tails", async () => {
    const state = fixture();
    const payload = "safe recovery event\n".repeat(110_000);
    for (const source of state.sources) fs.writeFileSync(source, payload);
    await captureSSHInterruptionLogs(state.info, state.backend, state.remote);
    expect(state.attach).toHaveBeenCalledTimes(4);
    const metadata = state.metadata();
    expect(metadata.captures).toHaveLength(3);
    for (const capture of metadata.captures) {
      expect(capture.truncated).toBe(true);
      expect(capture.originalBytes).toBe(Buffer.byteLength(payload));
      expect(capture.readBytes).toBeLessThanOrEqual(metadata.perLogByteCap);
      expect(capture.attachedBytes).toBeGreaterThan(0);
      expect(capture.error).toBeUndefined();
    }
    const retainedFiles = [
      ...fs
        .readdirSync(state.output)
        .filter((name) => name !== "attachments")
        .map((name) => path.join(state.output, name)),
      ...fs.readdirSync(state.copies).map((name) => path.join(state.copies, name)),
    ];
    expect(retainedFiles).toHaveLength(8);
    const bytes = retainedFiles.reduce((sum, file) => sum + fs.statSync(file).size, 0);
    expect(bytes).toBeLessThanOrEqual(8 * 1024 * 1024);
    for (const name of fs.readdirSync(state.copies)) {
      const original = name === "ssh-diagnostics" ? "ssh-diagnostics.json" : name;
      expect(
        fs
          .readFileSync(path.join(state.copies, name))
          .equals(fs.readFileSync(path.join(state.output, original))),
      ).toBe(true);
    }
  });

  it("retains the newest failure event when redaction expands short credential lines", async () => {
    const state = fixture();
    const newest = "newest failure event: SSH stream acquisition rejected";
    const payload = "password=Q7\n".repeat(70_000) + newest + "\n";
    for (const source of state.sources) fs.writeFileSync(source, payload);
    await captureSSHInterruptionLogs(state.info, state.backend, state.remote);
    const metadata = state.metadata();
    for (const capture of metadata.captures) {
      expect(capture.originalBytes).toBeLessThan(metadata.perLogByteCap);
      expect(capture.readBytes).toBe(capture.originalBytes);
      expect(capture.truncated).toBe(true);
      const retained = fs.readFileSync(path.join(state.output, capture.name), "utf8");
      expect(retained.endsWith(newest + "\n")).toBe(true);
      expect(retained).not.toContain("Q7");
      expect(retained).not.toContain("password=");
      expect(Buffer.byteLength(retained)).toBe(capture.attachedBytes);
    }
    const retainedBytes = [state.output, state.copies].reduce(
      (sum, directory) =>
        sum +
        fs
          .readdirSync(directory)
          .filter((name) => name !== "attachments")
          .reduce((bytes, name) => bytes + fs.statSync(path.join(directory, name)).size, 0),
      0,
    );
    expect(retainedBytes).toBeLessThanOrEqual(8 * 1024 * 1024);
  });

  it("redacts credential forms while retaining useful recovery events", async () => {
    const state = fixture();
    const payload = [
      "retained recovery event",
      `Authorization: Bearer ${credential}`,
      "Basic dXNlcjpwYXNz",
      "  bAsIc\tdXNlcjpwYXNz\r",
      `{"token":"${credential}"}`,
      `{\\"auth_token\\":\\"${credential}\\"}`,
      `--password ${credential}`,
      `AWS_SECRET_ACCESS_KEY=${credential}`,
      `{"clientSecretAccessKey":"${credential}"}`,
      `service.authTokenValue = ${credential}`,
      `export MY_PRIVATE_KEY_CONTENT=${credential}`,
      `wss://user:${credential}@localhost/path?access=${credential}`,
      "-----BEGIN PRIVATE KEY-----",
      credential,
      "-----END PRIVATE KEY-----",
      "QUJDREVGR0hJSktMTU5PUA==",
      "retained final event",
    ].join("\n");
    for (const source of state.sources) fs.writeFileSync(source, payload);
    await captureSSHInterruptionLogs(state.info, state.backend, state.remote);
    for (const name of [
      "ssh-backend-structured.log",
      "ssh-backend-process.log",
      "ssh-remote-agentctl.log",
    ]) {
      const captured = fs.readFileSync(path.join(state.output, name), "utf8");
      expect(captured).toContain("retained recovery event");
      expect(captured).toContain("retained final event");
      expect(captured).not.toContain(credential);
      for (const key of [
        "AWS_SECRET_ACCESS_KEY",
        "clientSecretAccessKey",
        "service.authTokenValue",
        "MY_PRIVATE_KEY_CONTENT",
      ]) {
        expect(captured).not.toContain(key);
      }
      expect(captured).not.toContain("dXNlcjpwYXNz");
      expect(captured).not.toContain("QUJDREVGR0hJSktMTU5PUA==");
      expect(captured).not.toContain("wss://");
    }
  });

  it("discards a partial first line whose credential marker fell outside the bounded tail", async () => {
    const state = fixture();
    const payload = `password=${credential.repeat(70_000)}\nretained final event\n`;
    fs.writeFileSync(state.backend.logPath, payload);
    await captureSSHInterruptionLogs(state.info, state.backend, state.remote);
    expect(state.metadata().captures[0].truncated).toBe(true);
    expect(fs.readFileSync(path.join(state.output, "ssh-backend-structured.log"), "utf8")).toBe(
      "retained final event\n",
    );
  });

  it("records missing and symlink input errors without reading their targets", async () => {
    const state = fixture();
    fs.unlinkSync(state.backend.logPath);
    fs.unlinkSync(state.remote);
    fs.symlinkSync(path.join(state.root, "backend-process.log"), state.remote);
    await expect(
      captureSSHInterruptionLogs(state.info, state.backend, state.remote),
    ).resolves.toBeUndefined();
    const captures = state.metadata().captures;
    expect(captures[0].error).toBe("ENOENT");
    expect(captures[2].error).toBe("ELOOP");
    expect(fs.existsSync(path.join(state.output, "ssh-remote-agentctl.log"))).toBe(false);
    expect(captures[1].attachedBytes).toBeGreaterThan(0);
  });

  it("captures a remote tail when its host mount is unavailable and redacts before writing", async () => {
    const state = fixture();
    fs.unlinkSync(state.remote);
    const payload = `${credential}\nAuthorization: Bearer ${credential}\nremote listener stopped\n`;
    const target = fakeDocker(state.root, `process.stdout.write(${JSON.stringify(payload)});`);
    const readRemote = vi.fn((byteLimit: number) => {
      expect(byteLimit).toBeGreaterThan(0);
      expect(byteLimit).toBeLessThan(2 * 1024 * 1024);
      return execInContainer(target, ["cat", state.remote], 1_000);
    });
    await captureSSHInterruptionLogs(state.info, state.backend, readRemote);
    expect(readRemote).toHaveBeenCalledOnce();
    const retained = fs.readFileSync(path.join(state.output, "ssh-remote-agentctl.log"), "utf8");
    expect(retained).toContain("remote listener stopped");
    expect(retained).not.toContain(credential);
    expect(state.metadata().captures[2].error).toBeUndefined();
    expect(state.metadata().captures[2].originalBytes).toBeUndefined();
  });

  it.each(["initial", "retry"])(
    "stops stalled %s Docker diagnostic reads without retaining partial output",
    async (attempt) => {
      const state = fixture();
      const retryMarker = path.join(state.root, "retry-marker");
      const target = fakeDocker(
        state.root,
        [
          'const fs = require("node:fs");',
          `const marker = ${JSON.stringify(retryMarker)};`,
          `if (${attempt === "retry"} && !fs.existsSync(marker)) {`,
          '  fs.writeFileSync(marker, "first attempt");',
          '  process.stderr.write("unable to upgrade to tcp: 409");',
          "  process.exit(1);",
          "}",
          `process.stdout.write(${JSON.stringify(`password=${credential}\n`)});`,
          `process.stderr.write(${JSON.stringify(`Authorization: Bearer ${credential}\n`)});`,
          'fs.writeFileSync(marker, "stalled after output");',
          'process.on("SIGTERM", () => {});',
          // The fake transport stalls, then exits if the subprocess deadline is absent.
          "setTimeout(() => process.exit(0), 5_000);",
        ].join("\n"),
      );
      const errors = [{ message: "original interruption assertion failed" }];
      Object.assign(state.info, { errors });
      const writes = vi.spyOn(fs, "writeFileSync");
      const started = performance.now();

      await captureSSHInterruptionLogs(state.info, state.backend, () =>
        execInContainer(target, ["cat", state.remote], 1_000),
      );

      expect(performance.now() - started).toBeLessThan(4_000);
      expect(fs.readFileSync(retryMarker, "utf8")).toBe("stalled after output");
      expect(state.metadata().captures[2].error).toBe("diagnostic collection failed");
      expect(fs.existsSync(path.join(state.output, "ssh-remote-agentctl.log"))).toBe(false);
      expect(state.metadata().captures[0].attachedBytes).toBeGreaterThan(0);
      expect(state.info.status).toBe("failed");
      expect(state.info.errors).toBe(errors);
      for (const [, body] of writes.mock.calls) expect(String(body)).not.toContain(credential);
    },
    15_000,
  );

  it("does not read the remote container after a passing test", async () => {
    const state = fixture();
    const readRemote = vi.fn(() => "remote listener stopped\n");
    await captureSSHInterruptionLogs(
      { ...state.info, status: "passed" },
      state.backend,
      readRemote,
    );
    expect(readRemote).not.toHaveBeenCalled();
    expect(state.attach).not.toHaveBeenCalled();
  });

  it("omits raw remote command failures from diagnostic metadata", async () => {
    const state = fixture();
    await captureSSHInterruptionLogs(state.info, state.backend, () => {
      throw new Error(`docker exec failed: password=${credential}`);
    });
    const metadata = state.metadata();
    expect(metadata.captures[2].error).toBe("diagnostic collection failed");
    expect(JSON.stringify(metadata)).not.toContain(credential);
    expect(state.info.status).toBe("failed");
  });

  it("preserves the original failure and closes log descriptors when attachment fails", async () => {
    const state = fixture();
    const originalFailure = new Error("original restart assertion failed");
    const errors = [{ message: originalFailure.message }];
    Object.assign(state.info, { errors });
    state.attach.mockRejectedValue(new Error("attachment storage unavailable"));
    const open = vi.spyOn(fs, "openSync");
    const close = vi.spyOn(fs, "closeSync");
    await expect(
      captureSSHInterruptionLogs(state.info, state.backend, state.remote),
    ).resolves.toBeUndefined();
    expect(state.info.status).toBe("failed");
    expect(state.info.errors).toBe(errors);
    expect(state.info.errors).toEqual([{ message: originalFailure.message }]);
    const logDescriptors = open.mock.calls.flatMap((args, index) =>
      state.sources.includes(String(args[0])) && open.mock.results[index].type === "return"
        ? [open.mock.results[index].value]
        : [],
    );
    expect(logDescriptors).toHaveLength(3);
    for (const fd of logDescriptors) expect(close).toHaveBeenCalledWith(fd);
    expect(state.metadata().captures.every((capture: { error?: string }) => capture.error)).toBe(
      true,
    );
  });
});
