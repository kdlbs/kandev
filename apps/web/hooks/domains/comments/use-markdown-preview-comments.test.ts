import { describe, expect, it } from "vitest";
import { isIgnoredTarget } from "./use-markdown-preview-comments";

describe("isIgnoredTarget", () => {
  it("treats SVG descendants inside interactive elements as ignored", () => {
    const button = document.createElement("button");
    button.innerHTML = "<svg><path /></svg>";

    expect(isIgnoredTarget(button.querySelector("path"))).toBe(true);
  });

  it("does not ignore plain non-interactive elements", () => {
    expect(isIgnoredTarget(document.createElement("span"))).toBe(false);
  });
});
