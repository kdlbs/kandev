import { describe, it, expect } from "vitest";
import { KEY_SEQUENCES } from "./key-sequences";

describe("KEY_SEQUENCES", () => {
  it("maps Ctrl+C to \\x03", () => {
    expect(KEY_SEQUENCES.ctrlC).toBe("\x03");
  });

  it("maps Ctrl+D to \\x04", () => {
    expect(KEY_SEQUENCES.ctrlD).toBe("\x04");
  });

  it("maps Esc / Tab", () => {
    expect(KEY_SEQUENCES.esc).toBe("\x1b");
    expect(KEY_SEQUENCES.tab).toBe("\t");
  });

  it("maps arrow keys", () => {
    expect(KEY_SEQUENCES.up).toBe("\x1b[A");
    expect(KEY_SEQUENCES.down).toBe("\x1b[B");
    expect(KEY_SEQUENCES.right).toBe("\x1b[C");
    expect(KEY_SEQUENCES.left).toBe("\x1b[D");
  });

  it("maps Home / End / PgUp / PgDn", () => {
    expect(KEY_SEQUENCES.home).toBe("\x01");
    expect(KEY_SEQUENCES.end).toBe("\x05");
    expect(KEY_SEQUENCES.pageUp).toBe("\x1b[5~");
    expect(KEY_SEQUENCES.pageDown).toBe("\x1b[6~");
  });
});
