import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const MAX_REPORTED_STREAM_BYTES = 4_000;

type FixtureCommandOptions = {
  timeout?: number;
};

type ChildProcessResult = {
  error?: Error | null;
  signal?: NodeJS.Signals | null;
  status?: number | null;
};

function processError(error: unknown): string | undefined {
  if (!(error instanceof Error))
    return error === undefined ? undefined : `process error: ${String(error)}`;
  const code = (error as NodeJS.ErrnoException).code;
  return `process error: ${typeof code === "string" ? code : error.message}`;
}

function outputTail(name: string, filePath: string): string {
  let fd: number | undefined;
  try {
    fd = fs.openSync(filePath, "r");
    const size = fs.fstatSync(fd).size;
    if (size === 0) return "";

    const byteCount = Math.min(size, MAX_REPORTED_STREAM_BYTES);
    const buffer = Buffer.alloc(byteCount);
    fs.readSync(fd, buffer, 0, byteCount, size - byteCount);
    const output = buffer.toString("utf8").trim();
    if (!output) return "";
    if (size <= MAX_REPORTED_STREAM_BYTES) return `${name}:\n${output}`;

    return `${name} (last ${output.length} characters; ${size - byteCount} bytes truncated):\n${output}`;
  } catch {
    return "";
  } finally {
    if (fd !== undefined) fs.closeSync(fd);
  }
}

function commandFailure(
  file: string,
  args: string[],
  result?: ChildProcessResult,
  invocationError?: unknown,
  outputFiles?: { stdout: string; stderr: string },
): Error {
  const details = [
    typeof result?.status === "number" ? `exit code: ${result.status}` : "",
    result?.error ? processError(result.error) : (processError(invocationError) ?? ""),
    typeof result?.signal === "string" ? `signal: ${result.signal}` : "",
    outputFiles ? outputTail("stderr", outputFiles.stderr) : "",
    outputFiles ? outputTail("stdout", outputFiles.stdout) : "",
  ].filter(Boolean);
  return new Error([`Command failed: ${[file, ...args].join(" ")}`, ...details].join("\n"));
}

export function runFixtureCommand(
  file: string,
  args: string[],
  options: FixtureCommandOptions = {},
): void {
  if (process.env.E2E_DEBUG) {
    const result = spawnSync(file, args, { ...options, stdio: "inherit" });
    if (result.error || result.status !== 0) {
      throw commandFailure(file, args, result);
    }
    return;
  }

  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-command-"));
  const stdoutPath = path.join(tempDir, "stdout");
  const stderrPath = path.join(tempDir, "stderr");
  const outputFiles = { stdout: stdoutPath, stderr: stderrPath };
  let stdoutFd: number | undefined;
  let stderrFd: number | undefined;
  let result: ReturnType<typeof spawnSync> | undefined;
  let invocationError: unknown;

  try {
    try {
      stdoutFd = fs.openSync(stdoutPath, "w");
      stderrFd = fs.openSync(stderrPath, "w");
      result = spawnSync(file, args, {
        ...options,
        stdio: ["ignore", stdoutFd, stderrFd],
      });
    } catch (error) {
      invocationError = error;
    } finally {
      if (stdoutFd !== undefined) {
        fs.closeSync(stdoutFd);
        stdoutFd = undefined;
      }
      if (stderrFd !== undefined) {
        fs.closeSync(stderrFd);
        stderrFd = undefined;
      }
    }
    if (invocationError || result?.error || result?.status !== 0) {
      throw commandFailure(file, args, result, invocationError, outputFiles);
    }
  } finally {
    if (stdoutFd !== undefined) fs.closeSync(stdoutFd);
    if (stderrFd !== undefined) fs.closeSync(stderrFd);
    fs.rmSync(tempDir, { recursive: true, force: true });
  }
}
