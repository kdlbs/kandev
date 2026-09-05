import { afterEach, describe, expect, it, vi } from "vitest";
import { renderPreviewSnapshot, rewritePreviewBlobUrls } from "./preview-surface";
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

describe("preview surface interaction safety", () => {
  it("does not cancel native text editing key events", () => {
    const host = document.createElement("div");
    const inputSnapshot: PreviewSnapshot = {
      ...snapshot,
      root: {
        ...snapshot.root,
        children: [
          {
            id: "input",
            tagName: "input",
            attributes: { id: "input" },
            styles: {},
            children: [],
            eventTypes: ["keydown"],
          },
        ],
      },
    };
    const onEvent = vi.fn<(event: PreviewEvent) => void>();
    document.body.append(host);
    renderPreviewSnapshot(host, inputSnapshot, onEvent);

    const event = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, key: "a" });
    expect(host.querySelector("input")?.dispatchEvent(event)).toBe(true);
    expect(event.defaultPrevented).toBe(false);
    expect(onEvent).toHaveBeenCalledWith({ type: "keydown", nodeId: "input", key: "a" });
  });

  it("prevents native form submission while leaving other defaults available", () => {
    const host = document.createElement("div");
    const formSnapshot: PreviewSnapshot = {
      ...snapshot,
      root: {
        ...snapshot.root,
        children: [
          {
            id: "form",
            tagName: "form",
            attributes: { id: "form" },
            styles: {},
            children: [],
            eventTypes: ["submit"],
          },
        ],
      },
    };
    document.body.append(host);
    renderPreviewSnapshot(host, formSnapshot, vi.fn());

    const event = new SubmitEvent("submit", { bubbles: true, cancelable: true });
    expect(host.querySelector("form")?.dispatchEvent(event)).toBe(false);
    expect(event.defaultPrevented).toBe(true);
  });

  it("rewrites longer blob tokens before their shorter prefixes", () => {
    const resourceUrls = new Map([
      ["blob:preview-runtime-1", "blob-url-1"],
      ["blob:preview-runtime-10", "blob-url-10"],
    ]);

    expect(rewritePreviewBlobUrls("url(blob:preview-runtime-10)", resourceUrls)).toBe(
      "url(blob-url-10)",
    );
  });
});
