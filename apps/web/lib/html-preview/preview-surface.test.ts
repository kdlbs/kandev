import { afterEach, describe, expect, it, vi } from "vitest";
import { renderPreviewSnapshot } from "./preview-surface";
import type { PreviewEvent, PreviewSnapshot } from "./preview-runtime-types";

const snapshot: PreviewSnapshot = {
  protocolVersion: 1,
  root: {
    id: "body",
    tagName: "body",
    attributes: { "data-runtime": "quickjs" },
    styles: {},
    children: [
      {
        id: "link",
        tagName: "a",
        attributes: { href: "https://example.com", id: "link" },
        styles: {},
        children: [
          {
            id: "link-text",
            tagName: "#text",
            attributes: {},
            styles: {},
            text: "Blocked",
            children: [],
            eventTypes: [],
          },
        ],
        eventTypes: ["click"],
      },
      {
        id: "button",
        tagName: "button",
        attributes: { id: "button" },
        styles: {},
        children: [
          {
            id: "button-text",
            tagName: "#text",
            attributes: {},
            styles: {},
            text: "Run",
            children: [],
            eventTypes: [],
          },
        ],
        eventTypes: ["click"],
      },
      {
        id: "script",
        tagName: "script",
        attributes: {},
        styles: {},
        text: "alert(1)",
        children: [],
        eventTypes: [],
      },
    ],
    eventTypes: [],
  },
  resources: [],
  diagnostics: [],
};

afterEach(() => {
  document.body.replaceChildren();
});

describe("preview surface", () => {
  it("renders data-only nodes without source HTML or navigation attributes", () => {
    const host = document.createElement("div");
    document.body.append(host);

    renderPreviewSnapshot(host, snapshot, vi.fn());

    expect(host.querySelector("script")).toBeNull();
    expect(host.querySelector("a")?.hasAttribute("href")).toBe(false);
    expect(host.textContent).toContain("Blocked");
  });

  it("sanitizes CSS inside style elements before adopting it", () => {
    const host = document.createElement("div");
    const styleSnapshot: PreviewSnapshot = {
      ...snapshot,
      root: {
        ...snapshot.root,
        children: [
          {
            id: "style",
            tagName: "style",
            attributes: {},
            styles: {},
            text: '@import "https://example.com/theme.css"; body { background: url("https://example.com/bg.png"); }',
            children: [],
            eventTypes: [],
          },
        ],
      },
    };
    document.body.append(host);

    renderPreviewSnapshot(host, styleSnapshot, vi.fn());

    const css = host.querySelector("style")?.textContent ?? "";
    expect(css).not.toContain("@import");
    expect(css).not.toContain("example.com");
  });

  it("preserves embedded data URLs while filtering srcset candidates", () => {
    const host = document.createElement("div");
    const resourceSnapshot: PreviewSnapshot = {
      ...snapshot,
      root: {
        ...snapshot.root,
        children: [
          {
            id: "srcset",
            tagName: "img",
            attributes: {
              srcset: "data:image/png;base64,abc 1x, https://example.com/image.png 2x",
            },
            styles: {},
            children: [],
            eventTypes: [],
          },
        ],
      },
    };
    document.body.append(host);

    renderPreviewSnapshot(host, resourceSnapshot, vi.fn());

    expect(host.querySelector("img")?.getAttribute("srcset")).toBe("data:image/png;base64,abc 1x");
  });

  it("bridges only supported user events to the runtime", () => {
    const host = document.createElement("div");
    const onEvent = vi.fn<(event: PreviewEvent) => void>();
    document.body.append(host);

    renderPreviewSnapshot(host, snapshot, onEvent);
    host.querySelector("button")?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

    expect(onEvent).toHaveBeenCalledWith({ type: "click", nodeId: "button" });
  });
});
