import { describe, expect, it } from "vitest";
import { detectRenderingEngine, markRenderingEngine } from "./rendering-engine";

type NavigatorFixture = {
  userAgent: string;
};

const safariMac: NavigatorFixture = {
  userAgent:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
};

describe("detectRenderingEngine", () => {
  it.each([
    ["macOS Safari", safariMac],
    [
      "macOS WKWebView",
      {
        userAgent:
          "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko)",
      },
    ],
    [
      "WebKitGTK",
      {
        userAgent:
          "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/617.1 (KHTML, like Gecko) Version/18.0 Safari/617.1",
      },
    ],
    [
      "iOS Chrome",
      {
        userAgent:
          "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/125.0.6422.99 Mobile/15E148 Safari/604.1",
      },
    ],
    [
      "iOS Edge",
      {
        userAgent:
          "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) EdgiOS/125.0.2535.85 Mobile/15E148 Safari/604.1",
      },
    ],
    [
      "iPadOS desktop mode",
      {
        userAgent:
          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
      },
    ],
  ] satisfies Array<[string, NavigatorFixture]>)("%s is WebKit", (_name, navigatorLike) => {
    expect(detectRenderingEngine(navigatorLike)).toBe("webkit");
  });

  it.each([
    [
      "desktop Chrome",
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
    ],
    [
      "Edge WebView2",
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0",
    ],
    [
      "Android Chrome",
      "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36",
    ],
    [
      "Firefox",
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.5; rv:127.0) Gecko/20100101 Firefox/127.0",
    ],
    ["unknown", "TestBrowser/1.0"],
  ] satisfies Array<[string, string]>)("%s is not WebKit", (_name, userAgent) => {
    expect(detectRenderingEngine({ userAgent })).toBe("other");
  });

  it("keeps desktop Mac Chrome on the non-WebKit path", () => {
    expect(
      detectRenderingEngine({
        userAgent:
          "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
      }),
    ).toBe("other");
  });
});

describe("markRenderingEngine", () => {
  it("writes a transient WebKit marker to the supplied root", () => {
    const root = document.createElement("html");

    expect(markRenderingEngine(root, safariMac)).toBe("webkit");
    expect(root.dataset.renderingEngine).toBe("webkit");
  });

  it("fails safe to the default marker for missing navigator data", () => {
    const root = document.createElement("html");

    expect(markRenderingEngine(root, null)).toBe("other");
    expect(root.dataset.renderingEngine).toBe("other");
  });
});
