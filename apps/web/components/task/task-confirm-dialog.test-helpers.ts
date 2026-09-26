import { expect } from "vitest";

export function expectCompactWarning(warning: HTMLElement) {
  expect(warning.getAttribute("role")).toBe("alert");
  expect(warning.className).toContain("gap-1.5");
  expect(warning.className).toContain("p-2.5");
  expect(warning.className).toContain("text-xs");
  expect(warning.className).toContain("leading-5");
  expect(warning.className).toContain("text-pretty");
  expect(warning.querySelector("svg")?.getAttribute("class")).toContain("h-3.5");
}
