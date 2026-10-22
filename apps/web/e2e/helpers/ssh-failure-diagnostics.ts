import fs from "node:fs";
import path from "node:path";
import type { TestInfo } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";

const TOTAL_BYTES = 8 * 1024 * 1024;
const METADATA_BYTES = 64 * 1024;
// Playwright retains both outputPath originals and attachment copies. Budget for both.
const LOG_BYTES = Math.floor((TOTAL_BYTES - METADATA_BYTES) / 6);

type Capture = {
  name: string;
  originalBytes?: number;
  readBytes?: number;
  attachedBytes?: number;
  truncated?: boolean;
  error?: string;
};

function errorCode(error: unknown): string {
  return (error as NodeJS.ErrnoException)?.code ?? "diagnostic collection failed";
}

function redactLog(text: string): string {
  return text
    .replace(
      /-----BEGIN [^-]*PRIVATE KEY-----[\s\S]*?(?:-----END [^-]*PRIVATE KEY-----|$)/g,
      "[REDACTED KEY]",
    )
    .split("\n")
    .map((line) =>
      /\bbasic\s|bearer\s|authorization|(?:token|secret|password|private[_-]?key|api[_-]?key)[A-Za-z0-9_.-]*[\\"'\s]*[:=]|--(?:auth-token|token|password|api-key)\b/i.test(
        line,
      ) || /^[A-Za-z0-9+/=]{8,}\r?$/.test(line)
        ? "[REDACTED credential-bearing line]"
        : line.replace(/(?:https?|wss?):\/\/[^\s"\\]+/gi, "[REDACTED URL]"),
    )
    .join("\n");
}

async function attachLog(testInfo: TestInfo, source: string, capture: Capture): Promise<void> {
  let fd: number | undefined;
  try {
    fd = fs.openSync(source, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW);
    const stat = fs.fstatSync(fd);
    if (!stat.isFile()) throw new Error("not a regular log file");
    const bytes = Math.min(stat.size, LOG_BYTES);
    const offset = stat.size - bytes;
    const buffer = Buffer.alloc(bytes);
    const read = fs.readSync(fd, buffer, 0, bytes, offset);
    let text = buffer.subarray(0, read).toString("utf8");
    // A tail can split a credential from its marker. Discard the partial first line.
    if (offset > 0) {
      const newline = text.indexOf("\n");
      text = newline === -1 ? "" : text.slice(newline + 1);
    }
    const body = Buffer.from(redactLog(text));
    capture.originalBytes = stat.size;
    capture.readBytes = read;
    const retained = body.subarray(Math.max(0, body.length - LOG_BYTES));
    capture.attachedBytes = retained.length;
    capture.truncated = offset > 0 || body.length > LOG_BYTES;
    const output = testInfo.outputPath(capture.name);
    fs.writeFileSync(output, retained);
    await testInfo.attach(capture.name, { path: output, contentType: "text/plain" });
  } catch (error) {
    capture.error = errorCode(error);
  } finally {
    if (fd !== undefined) {
      try {
        fs.closeSync(fd);
      } catch {
        /* Preserve the original test failure. */
      }
    }
  }
}

/** Save only the known fixture logs, before SSH runtime teardown, on unexpected failure. */
export async function captureSSHInterruptionLogs(
  testInfo: TestInfo,
  backend: Pick<BackendContext, "logPath" | "tmpDir">,
  remoteLog?: string,
): Promise<void> {
  if (
    testInfo.status === testInfo.expectedStatus ||
    !["failed", "timedOut", "interrupted"].includes(testInfo.status ?? "")
  )
    return;
  try {
    const logs = [
      { source: backend.logPath, name: "ssh-backend-structured.log" },
      { source: path.join(backend.tmpDir, "backend-process.log"), name: "ssh-backend-process.log" },
      { source: remoteLog, name: "ssh-remote-agentctl.log" },
    ];
    const captures: Capture[] = [];
    for (const { source, name } of logs) {
      const capture: Capture = { name };
      captures.push(capture);
      if (source) await attachLog(testInfo, source, capture);
      else capture.error = "remote log identity unavailable";
    }
    const output = testInfo.outputPath("ssh-diagnostics.json");
    fs.writeFileSync(
      output,
      JSON.stringify({ totalByteCap: TOTAL_BYTES, perLogByteCap: LOG_BYTES, captures }, null, 2),
    );
    await testInfo.attach("ssh-diagnostics", { path: output, contentType: "application/json" });
  } catch {
    // Diagnostics must never replace the original assertion or fixture failure.
  }
}
