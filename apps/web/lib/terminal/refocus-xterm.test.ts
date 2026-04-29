import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { refocusXtermTextarea } from "./refocus-xterm";

describe("refocusXtermTextarea", () => {
  let textarea: HTMLTextAreaElement;

  beforeEach(() => {
    textarea = document.createElement("textarea");
    textarea.className = "xterm-helper-textarea";
    document.body.appendChild(textarea);
  });

  afterEach(() => {
    textarea.remove();
  });

  it("focuses the xterm helper textarea", () => {
    const other = document.createElement("input");
    document.body.appendChild(other);
    other.focus();
    expect(document.activeElement).toBe(other);

    refocusXtermTextarea();
    expect(document.activeElement).toBe(textarea);
    other.remove();
  });

  it("does not throw when no xterm textarea is mounted", () => {
    textarea.remove();
    expect(() => refocusXtermTextarea()).not.toThrow();
  });
});
