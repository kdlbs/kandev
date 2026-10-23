import { describe, expect, it } from "vitest";
import { nativeMarkdownReplacement, projectMarkdownInput } from "./markdown-input-context";

describe("canonical native projection", () => {
  it("normalizes only newlines and maps UTF-16 boundaries back to exact source", () => {
    const projection = projectMarkdownInput("\uFEFFA\r\n😀\nZ\rQ");
    expect(projection.value).toBe("\uFEFFA\n😀\nZ\nQ");
    expect(Array.from({ length: 10 }, (_, i) => projection.toSource(i))).toEqual([
      0, 1, 2, 4, 5, 6, 7, 8, 9, 10,
    ]);
    expect(projection.toNative(4)).toBe(3);
  });
  it.each([
    ["a\r\nb\n", "a\nnew\nb\n", { start: 3, end: 3, text: "new\n" }],
    ["a\r\nb", "a\nb\nnext", { start: 4, end: 4, text: "\r\nnext" }],
    ["\uFEFFa😀\r\nb\n", "\uFEFFa😎\nb\n", { start: 3, end: 4, text: "\ude0e" }],
  ])(
    "uses the edited line style while retaining untouched source: %j",
    (source, value, expected) => {
      expect(nativeMarkdownReplacement(source, value)).toEqual(expected);
    },
  );
});

// Selection, source and other editor instances must not share a transport.
describe("native transport selection", () => {
  it("uses scoped textarea input when the native constructor throws without changing globals", async () => {
    const { createMarkdownInput } = await import("./markdown-input");
    const original = Object.getOwnPropertyDescriptor(globalThis, "EditContext");
    class RejectedContext {
      constructor() {
        throw new Error("Rejected native constructor");
      }
    }
    Object.defineProperty(globalThis, "EditContext", {
      configurable: true,
      value: RejectedContext,
    });
    const host = document.createElement("div");
    const input = createMarkdownInput(
      { host, text: "exact\r\n", getCaretRect: () => undefined, onSelection: () => {} },
      "Markdown content",
    );
    try {
      input.attach(host);
      expect(host.shadowRoot!.querySelector("textarea")).not.toBeNull();
      expect(Reflect.get(globalThis, "EditContext")).toBe(RejectedContext);
      expect(input.context.text).toBe("exact\r\n");
    } finally {
      input.dispose();
      if (original) Object.defineProperty(globalThis, "EditContext", original);
      else Reflect.deleteProperty(globalThis, "EditContext");
    }
  });
});
