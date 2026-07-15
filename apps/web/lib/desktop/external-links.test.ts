import { afterEach, describe, expect, it, vi } from "vitest";
import {
  classifyDesktopLink,
  createDesktopExternalLinkAdapter,
  subscribeDesktopExternalLinks,
} from "./external-links";

const BASE_URL = "http://127.0.0.1:4100/tasks/one";
const EXTERNAL_URL = "https://github.com/kdlbs/kandev";

afterEach(() => {
  document.body.replaceChildren();
});

describe("classifyDesktopLink", () => {
  it.each([
    "/settings/general",
    "http://127.0.0.1:4200/preview",
    "http://localhost:4300/forwarded",
    "http://dev.localhost:4300/forwarded",
    "http://[::1]:4300/forwarded",
    "http://[::ffff:127.0.0.1]:4300/forwarded",
    "http://[::ffff:0.0.0.0]:4300/forwarded",
    "http://0.0.0.0:4300/forwarded",
    "http://[::]:4300/forwarded",
    "blob:http://127.0.0.1:4100/asset",
  ])("keeps %s in the WebView", (url) => {
    expect(classifyDesktopLink(url, BASE_URL)).toBe("webview");
  });

  it.each([EXTERNAL_URL, "http://example.com/docs", "mailto:support@example.com?subject=Kandev"])(
    "routes %s to the system handler",
    (url) => {
      expect(classifyDesktopLink(url, BASE_URL)).toBe("external");
    },
  );

  it.each([
    "javascript:alert(1)",
    "file:///tmp/private",
    "ssh://example.com/repository",
    "mailto:",
    "https://user:secret@example.com/private",
    "http://[invalid",
  ])("blocks unsafe desktop destination %s", (url) => {
    expect(classifyDesktopLink(url, BASE_URL)).toBe("blocked");
  });
});

describe("desktop external-link adapter", () => {
  it("invokes the bounded native command for an external URL", async () => {
    const invoke = vi.fn().mockResolvedValue(undefined);
    const openWindow = vi.fn();
    const adapter = createDesktopExternalLinkAdapter(
      { isAvailable: () => true, invoke },
      { baseUrl: () => BASE_URL, openWindow },
    );

    await expect(adapter.open(EXTERNAL_URL)).resolves.toBe("external");

    expect(invoke).toHaveBeenCalledWith("open_external_url", {
      url: EXTERNAL_URL,
    });
    expect(openWindow).not.toHaveBeenCalled();
  });

  it("resolves protocol-relative external URLs before invoking native code", async () => {
    const invoke = vi.fn().mockResolvedValue(undefined);
    const adapter = createDesktopExternalLinkAdapter(
      { isAvailable: () => true, invoke },
      { baseUrl: () => BASE_URL, openWindow: vi.fn() },
    );

    await adapter.open("//example.com/docs");

    expect(invoke).toHaveBeenCalledWith("open_external_url", {
      url: "http://example.com/docs",
    });
  });

  it("preserves ordinary browser window.open behavior without Tauri", async () => {
    const invoke = vi.fn();
    const openWindow = vi.fn();
    const adapter = createDesktopExternalLinkAdapter(
      { isAvailable: () => false, invoke },
      { baseUrl: () => BASE_URL, openWindow },
    );

    await expect(adapter.open("custom://browser-owned")).resolves.toBe("browser");

    expect(openWindow).toHaveBeenCalledWith(
      "custom://browser-owned",
      "_blank",
      "noopener,noreferrer",
    );
    expect(invoke).not.toHaveBeenCalled();
  });

  it("fails closed for blocked desktop destinations", async () => {
    const invoke = vi.fn();
    const openWindow = vi.fn();
    const adapter = createDesktopExternalLinkAdapter(
      { isAvailable: () => true, invoke },
      { baseUrl: () => BASE_URL, openWindow },
    );

    await expect(adapter.open("https://user:secret@example.com/private")).resolves.toBe("blocked");

    expect(invoke).not.toHaveBeenCalled();
    expect(openWindow).not.toHaveBeenCalled();
  });
});

describe("subscribeDesktopExternalLinks", () => {
  function addAnchor(href: string, attributes: Record<string, string> = {}) {
    const anchor = document.createElement("a");
    anchor.href = href;
    anchor.target = "_blank";
    for (const [name, value] of Object.entries(attributes)) anchor.setAttribute(name, value);
    const child = document.createElement("span");
    anchor.append(child);
    document.body.append(anchor);
    return { anchor, child };
  }

  function setupDesktop() {
    const invoke = vi.fn().mockResolvedValue(undefined);
    const adapter = createDesktopExternalLinkAdapter(
      { isAvailable: () => true, invoke },
      { baseUrl: () => BASE_URL, openWindow: vi.fn() },
    );
    const unsubscribe = subscribeDesktopExternalLinks(document, adapter);
    return { invoke, unsubscribe };
  }

  it("intercepts nested clicks on safe external target=_blank anchors", async () => {
    const { invoke, unsubscribe } = setupDesktop();
    const { child } = addAnchor(EXTERNAL_URL);
    const event = new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 });

    child.dispatchEvent(event);
    await Promise.resolve();

    expect(event.defaultPrevented).toBe(true);
    expect(invoke).toHaveBeenCalledWith("open_external_url", {
      url: EXTERNAL_URL,
    });
    unsubscribe();
  });

  it.each([
    ["same-origin", "/settings/general", {}],
    ["loopback", "http://localhost:9000/preview", {}],
    ["blob", "blob:http://127.0.0.1:4100/id", {}],
    ["download", "https://example.com/archive.zip", { download: "archive.zip" }],
  ])("leaves %s anchors to existing WebView behavior", async (_name, href, attributes) => {
    const { invoke, unsubscribe } = setupDesktop();
    const { anchor } = addAnchor(href, attributes);
    const event = new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 });

    anchor.dispatchEvent(event);
    await Promise.resolve();

    expect(event.defaultPrevented).toBe(false);
    expect(invoke).not.toHaveBeenCalled();
    unsubscribe();
  });

  it.each(["https://user:secret@example.com/private", "javascript:alert(1)"])(
    "prevents unsafe target %s from opening",
    async (href) => {
      const { invoke, unsubscribe } = setupDesktop();
      const { anchor } = addAnchor(href);
      const event = new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 });

      anchor.dispatchEvent(event);
      await Promise.resolve();

      expect(event.defaultPrevented).toBe(true);
      expect(invoke).not.toHaveBeenCalled();
      unsubscribe();
    },
  );

  it("does not intercept anchors when Tauri is unavailable", () => {
    const invoke = vi.fn();
    const adapter = createDesktopExternalLinkAdapter(
      { isAvailable: () => false, invoke },
      { baseUrl: () => BASE_URL, openWindow: vi.fn() },
    );
    const unsubscribe = subscribeDesktopExternalLinks(document, adapter);
    const { anchor } = addAnchor("https://example.com/docs");
    const event = new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 });

    anchor.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(invoke).not.toHaveBeenCalled();
    unsubscribe();
  });

  it("respects application handlers that cancel an external link", async () => {
    const { invoke, unsubscribe } = setupDesktop();
    const { anchor } = addAnchor(EXTERNAL_URL);
    anchor.addEventListener("click", (event) => event.preventDefault());
    const event = new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 });

    anchor.dispatchEvent(event);
    await Promise.resolve();

    expect(invoke).not.toHaveBeenCalled();
    unsubscribe();
  });
});
