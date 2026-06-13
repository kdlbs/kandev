import { afterAll, describe, expect, it } from "vitest";

import { __resetBrokenPipeGuard, ignoreBrokenPipe } from "./process";

function makeError(code: string, message: string): NodeJS.ErrnoException {
  return Object.assign(new Error(message), { code });
}

function lastErrorListener(stream: NodeJS.WriteStream): (err: NodeJS.ErrnoException) => void {
  const listeners = stream.listeners("error");
  return listeners[listeners.length - 1] as (err: NodeJS.ErrnoException) => void;
}

describe("ignoreBrokenPipe", () => {
  // Capture the handler we install so we can both exercise and clean it up.
  let installed: ((err: NodeJS.ErrnoException) => void) | undefined;

  afterAll(() => {
    if (installed) {
      process.stdout.removeListener("error", installed);
      process.stderr.removeListener("error", installed);
    }
    // Reset the module guard alongside listener removal so a later
    // ignoreBrokenPipe() reattaches instead of hitting a no-op with no
    // listeners present (order-independent for other tests in this worker).
    __resetBrokenPipeGuard();
  });

  it("attaches an error handler to stdout and stderr", () => {
    const stdoutBefore = process.stdout.listeners("error").length;
    const stderrBefore = process.stderr.listeners("error").length;

    ignoreBrokenPipe();

    expect(process.stdout.listeners("error").length).toBe(stdoutBefore + 1);
    expect(process.stderr.listeners("error").length).toBe(stderrBefore + 1);

    // stdout and stderr share the same handler reference.
    installed = lastErrorListener(process.stdout);
    expect(lastErrorListener(process.stderr)).toBe(installed);
  });

  it("swallows EPIPE errors without throwing", () => {
    const handler = lastErrorListener(process.stdout);
    expect(() => handler(makeError("EPIPE", "write EPIPE"))).not.toThrow();
  });

  it("rethrows non-EPIPE errors", () => {
    const handler = lastErrorListener(process.stdout);
    expect(() => handler(makeError("EACCES", "boom"))).toThrow("boom");
  });

  it("is idempotent across repeated calls", () => {
    const stdoutCount = process.stdout.listeners("error").length;
    const stderrCount = process.stderr.listeners("error").length;

    ignoreBrokenPipe();
    ignoreBrokenPipe();

    expect(process.stdout.listeners("error").length).toBe(stdoutCount);
    expect(process.stderr.listeners("error").length).toBe(stderrCount);
  });
});
