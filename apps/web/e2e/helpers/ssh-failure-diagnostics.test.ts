import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { TestInfo } from "@playwright/test";
import { afterEach, describe, expect, it, vi } from "vitest";
import { captureSSHInterruptionLogs } from "./ssh-failure-diagnostics";

const roots: string[] = [];
const credential = "FAKE-CREDENTIAL-DO-NOT-RETAIN";

afterEach(() => {
  vi.restoreAllMocks();
  for (const root of roots.splice(0)) fs.rmSync(root, { recursive: true, force: true });
});

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
