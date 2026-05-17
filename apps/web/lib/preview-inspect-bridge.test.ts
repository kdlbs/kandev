import { describe, expect, it } from "vitest";
import { formatAnnotations, type Annotation } from "./preview-inspect-bridge";

const pin: Annotation = {
  id: "a-1",
  number: 1,
  kind: "pin",
  pagePath: "/products",
  comment: "Make this primary color",
  element: {
    tag: "button",
    id: "submit",
    role: "button",
    ariaLabel: "Submit form",
    text: "Submit",
    selector: "div#root > button#submit",
  },
};

const area: Annotation = {
  id: "a-2",
  number: 2,
  kind: "area",
  pagePath: "/products",
  comment: "Add a New badge to featured items",
  rect: { x: 40, y: 200, w: 320, h: 180 },
  elements: [
    { tag: "div", classes: "product-card featured", selector: "div.product-card" },
    { tag: "h2", classes: "title", text: "Item", selector: "h2.title" },
  ],
};

describe("formatAnnotations", () => {
  it("returns empty string when no annotations", () => {
    expect(formatAnnotations([])).toBe("");
  });

  it("renders a single pin with element details and comment", () => {
    const out = formatAnnotations([pin]);
    expect(out).toContain("Preview annotations on `/products`");
    expect(out).toContain("1. [Pin] `button#submit`");
    expect(out).toContain('role="button"');
    expect(out).toContain('"Submit form"');
    expect(out).toContain("Comment: Make this primary color");
    expect(out).toContain("Selector: `div#root > button#submit`");
  });

  it("renders an area with bounding rect and contained elements", () => {
    const out = formatAnnotations([area]);
    expect(out).toContain("2. [Area 320x180 at (40,200)]");
    expect(out).toContain("Contains: `div.product-card`, `h2.title`");
    expect(out).toContain("Comment: Add a New badge to featured items");
  });

  it("renders multiple annotations in order under a single header", () => {
    const out = formatAnnotations([pin, area]);
    const headerCount = (out.match(/Preview annotations on/g) || []).length;
    expect(headerCount).toBe(1);
    expect(out.indexOf("1. [Pin]")).toBeLessThan(out.indexOf("2. [Area"));
  });

  it("omits comment line when comment is empty", () => {
    const noComment: Annotation = { ...pin, comment: "" };
    const out = formatAnnotations([noComment]);
    expect(out).not.toContain("Comment:");
  });

  it("groups annotations by pagePath when they differ", () => {
    const other: Annotation = { ...pin, id: "a-3", number: 3, pagePath: "/about" };
    const out = formatAnnotations([pin, other]);
    expect(out).toContain("Preview annotations on `/products`");
    expect(out).toContain("Preview annotations on `/about`");
  });
});
