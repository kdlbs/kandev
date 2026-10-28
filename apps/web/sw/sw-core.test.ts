import { describe, expect, it } from "vitest";
import {
  cacheNames,
  isCacheableAsset,
  isCacheablePrecache,
  notificationOptions,
  parsePushPayload,
  pickClientForUrl,
  routeRequest,
  sanitizeTargetUrl,
  shouldSuppressNotification,
  staleCacheNames,
} from "./sw-core";

const ORIGIN = "https://kandev.example";
const TASK_ONE = "/t/1";
const TASK_ONE_SESSION = "/t/1?sessionId=s";
const FALLBACK_TITLE = "Kandev";
const PRECACHE = new Set(["/offline.html", "/icon.svg"]);

function request(path: string, init: { method?: string; mode?: string } = {}) {
  return { method: init.method ?? "GET", mode: init.mode ?? "no-cors", url: `${ORIGIN}${path}` };
}

function response(overrides: Partial<Parameters<typeof isCacheableAsset>[0]> & { ct?: string }) {
  const { ct = "text/javascript", ...rest } = overrides;
  return {
    ok: true,
    status: 200,
    type: "basic",
    redirected: false,
    headers: { get: (name: string) => (name.toLowerCase() === "content-type" ? ct : null) },
    ...rest,
  };
}

describe("routeRequest", () => {
  it("never intercepts API, WebSocket, debug or MCP traffic", () => {
    for (const path of ["/api/v1/tasks", "/api", "/ws", "/ws/session", "/debug/vars", "/mcp"]) {
      expect(routeRequest(request(path), ORIGIN, PRECACHE)).toBe("passthrough");
      expect(routeRequest(request(path, { mode: "navigate" }), ORIGIN, PRECACHE)).toBe(
        "passthrough",
      );
    }
  });

  it("does not touch non-GET or cross-origin requests", () => {
    expect(routeRequest(request("/assets/a.js", { method: "POST" }), ORIGIN, PRECACHE)).toBe(
      "passthrough",
    );
    expect(
      routeRequest(
        { method: "GET", mode: "cors", url: "https://other.example/assets/a.js" },
        ORIGIN,
        PRECACHE,
      ),
    ).toBe("passthrough");
  });

  it("classifies navigations, hashed assets and precached files", () => {
    expect(routeRequest(request("/t/abc", { mode: "navigate" }), ORIGIN, PRECACHE)).toBe(
      "navigate",
    );
    expect(routeRequest(request("/assets/index-abc.js"), ORIGIN, PRECACHE)).toBe("asset");
    expect(routeRequest(request("/offline.html"), ORIGIN, PRECACHE)).toBe("precache");
    expect(routeRequest(request("/anything-else.json"), ORIGIN, PRECACHE)).toBe("passthrough");
  });
});

const HTML_TYPE = "text/html";

describe("isCacheableAsset", () => {
  it("accepts a plain successful script", () => {
    expect(isCacheableAsset(response({}))).toBe(true);
  });

  it("refuses redirects, opaque responses, errors and HTML (an access proxy login page)", () => {
    expect(isCacheableAsset(response({ redirected: true }))).toBe(false);
    expect(isCacheableAsset(response({ type: "opaque" }))).toBe(false);
    expect(isCacheableAsset(response({ type: "opaqueredirect", ok: false, status: 0 }))).toBe(
      false,
    );
    expect(isCacheableAsset(response({ ok: false, status: 404 }))).toBe(false);
    expect(isCacheableAsset(response({ ct: `${HTML_TYPE}; charset=utf-8` }))).toBe(false);
    expect(isCacheableAsset(response({ ct: "" }))).toBe(false);
  });
});

describe("isCacheablePrecache", () => {
  it("allows HTML but still refuses redirects", () => {
    expect(isCacheablePrecache(response({ ct: HTML_TYPE }))).toBe(true);
    expect(isCacheablePrecache(response({ ct: HTML_TYPE, redirected: true }))).toBe(false);
  });
});

describe("cache versioning", () => {
  it("deletes only older kandev caches and leaves foreign caches alone", () => {
    const current = cacheNames("v2");
    const existing = [
      current.shell,
      current.assets,
      "kandev-shell-v1",
      "kandev-assets-v1",
      "other",
    ];
    expect(staleCacheNames(existing, "v2")).toEqual(["kandev-shell-v1", "kandev-assets-v1"]);
  });
});

describe("parsePushPayload", () => {
  it("reads the documented fields", () => {
    expect(
      parsePushPayload(
        JSON.stringify({
          title: "T",
          body: "B",
          tag: "g",
          url: TASK_ONE_SESSION,
          taskId: "1",
          sessionId: "s",
        }),
        FALLBACK_TITLE,
      ),
    ).toEqual({
      title: "T",
      body: "B",
      tag: "g",
      url: TASK_ONE_SESSION,
      taskId: "1",
      sessionId: "s",
    });
  });

  it("degrades to a generic notification for empty or malformed messages", () => {
    for (const raw of [null, "", "not json", "[1]", "null", "42"]) {
      expect(parsePushPayload(raw, FALLBACK_TITLE)).toMatchObject({
        title: FALLBACK_TITLE,
        url: "/",
      });
    }
  });

  it("drops a non-local deep link", () => {
    expect(parsePushPayload('{"title":"x","url":"https://evil.example/"}', "K").url).toBe("/");
  });
});

describe("sanitizeTargetUrl", () => {
  it("only accepts origin-relative paths", () => {
    expect(sanitizeTargetUrl(TASK_ONE)).toBe(TASK_ONE);
    for (const bad of ["", "t/1", "//evil.example", "https://evil.example", "/\\evil"]) {
      expect(sanitizeTargetUrl(bad)).toBe("/");
    }
  });
});

describe("shouldSuppressNotification", () => {
  it("suppresses only when a window is both focused and visible", () => {
    const win = (focused: boolean, visibilityState: string) => ({
      url: `${ORIGIN}/`,
      focused,
      visibilityState,
    });
    expect(shouldSuppressNotification([win(true, "visible")])).toBe(true);
    expect(shouldSuppressNotification([win(false, "visible")])).toBe(false);
    expect(shouldSuppressNotification([win(true, "hidden")])).toBe(false);
    expect(shouldSuppressNotification([])).toBe(false);
  });
});

describe("notificationOptions", () => {
  it("carries the tag for collapsing and re-alerts on replacement", () => {
    const options = notificationOptions(
      { title: "T", body: "B", tag: "kandev-x", url: TASK_ONE, taskId: "1" },
      "/icon.png",
    );
    expect(options).toMatchObject({ tag: "kandev-x", renotify: true, data: { url: TASK_ONE } });
    expect(notificationOptions({ title: "T", body: "B", url: "/" }, "/icon.png").renotify).toBe(
      false,
    );
  });
});

describe("pickClientForUrl", () => {
  const client = (path: string, focused = false) => ({
    url: `${ORIGIN}${path}`,
    focused,
    visibilityState: "visible",
  });

  it("prefers a client already on the target, then a focused one, then any", () => {
    const a = client("/");
    const b = client(TASK_ONE_SESSION);
    const c = client("/tasks", true);
    expect(pickClientForUrl([a, b, c], TASK_ONE_SESSION, ORIGIN)).toBe(b);
    expect(pickClientForUrl([a, c], "/t/2", ORIGIN)).toBe(c);
    expect(pickClientForUrl([a], "/t/2", ORIGIN)).toBe(a);
    expect(pickClientForUrl([], "/t/2", ORIGIN)).toBeUndefined();
  });
});
