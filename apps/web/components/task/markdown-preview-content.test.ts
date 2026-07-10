import { describe, expect, it } from "vitest";
import { isInteractiveSourceClickTarget } from "./markdown-preview-content";

describe("isInteractiveSourceClickTarget", () => {
  it("treats SVG descendants inside interactive elements as interactive", () => {
    const button = document.createElement("button");
    button.innerHTML = "<svg><path /></svg>";

    expect(isInteractiveSourceClickTarget(button.querySelector("path"))).toBe(true);
  });

  it("ignores plain non-interactive elements", () => {
    expect(isInteractiveSourceClickTarget(document.createElement("span"))).toBe(false);
  });
});
