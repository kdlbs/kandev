import { describe, it, expect } from "vitest";
import { KEY_SEQUENCES, ctrlChord } from "./key-sequences";

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

describe("ctrlChord", () => {
  it("returns \\x03 for 'c' (Ctrl+C)", () => {
    expect(ctrlChord("c")).toBe("\x03");
  });

  it("is case-insensitive — 'C' == 'c'", () => {
    expect(ctrlChord("C")).toBe("\x03");
  });

  it("returns \\x01 for 'a' (Ctrl+A / Home)", () => {
    expect(ctrlChord("a")).toBe("\x01");
  });

  it("returns \\x1a for 'z' (Ctrl+Z)", () => {
    expect(ctrlChord("z")).toBe("\x1a");
  });

  it("returns null for non-letter characters", () => {
    expect(ctrlChord("1")).toBeNull();
    expect(ctrlChord(" ")).toBeNull();
    expect(ctrlChord("!")).toBeNull();
  });

  it("returns null for multi-char / empty input", () => {
    expect(ctrlChord("")).toBeNull();
    expect(ctrlChord("ab")).toBeNull();
  });
});
