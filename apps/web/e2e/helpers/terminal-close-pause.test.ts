import { describe, expect, it } from "vitest";
import { partitionTerminalDestroyRequest } from "../tests/terminal/terminal-close-pause";

describe("partitionTerminalDestroyRequest", () => {
  it("pauses only the destroy frame in a newline-delimited batch", () => {
    const before = JSON.stringify({ type: "request", action: "terminal.input", id: "before" });
    const destroy = JSON.stringify({ type: "request", action: "user_shell.destroy", id: "close" });
    const after = JSON.stringify({ type: "request", action: "terminal.input", id: "after" });

    expect(partitionTerminalDestroyRequest(`${before}\n${destroy}\n${after}\n`)).toEqual({
      destroyFrame: destroy,
      passthrough: `${before}\n${after}\n`,
    });
  });

  it("does not partition unrelated or binary messages", () => {
    const unrelated = JSON.stringify({ type: "event", action: "user_shell.destroy" });

    expect(partitionTerminalDestroyRequest(unrelated)).toBeNull();
    expect(partitionTerminalDestroyRequest(Buffer.from(unrelated))).toBeNull();
  });
});
