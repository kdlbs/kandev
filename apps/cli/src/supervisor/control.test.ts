import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { afterEach, describe, expect, it } from "vitest";
import { requestRestart, startControlServer } from "./control";

const tmpDirs: string[] = [];

afterEach(() => {
  for (const dir of tmpDirs.splice(0)) {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

describe("supervisor control protocol", () => {
  it("accepts restart requests and calls the restart handler asynchronously", async () => {
    let calls = 0;
    const socket = testSocketPath();
    const server = await startControlServer(socket, async () => {
      calls += 1;
    });

    const response = await requestRestart(socket);
    await waitFor(() => calls === 1);
    await server.close();

    expect(response).toEqual({ accepted: true, message: "Restart accepted" });
  });
});

function testSocketPath(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-supervisor-control-"));
  tmpDirs.push(dir);
  if (process.platform === "win32") {
    return `\\\\.\\pipe\\kandev-test-${process.pid}-${Date.now()}`;
  }
  return path.join(dir, "control.sock");
}

async function waitFor(predicate: () => boolean): Promise<void> {
  const deadline = Date.now() + 1000;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("condition not met");
}
