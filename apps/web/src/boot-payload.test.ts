import { describe, expect, it, vi } from "vitest";
import { loadBootPayload, readBootPayload, readBootToken } from "./boot-payload";

const JIRA_BUNDLE_URL = "/api/plugins/jira/bundle";

describe("readBootToken", () => {
  it("returns the per-boot operator token from runtime.csrfToken", () => {
    const win = {
      __KANDEV_BOOT_PAYLOAD__: {
        version: 1,
        runtime: { apiPrefix: "/api/v1", webSocketPath: "/ws", csrfToken: "boot-token-abc" },
      },
    } as unknown as Window;

    expect(readBootToken(win)).toBe("boot-token-abc");
  });

  it("returns undefined when the payload or token is absent", () => {
    expect(readBootToken({} as Window)).toBeUndefined();
    expect(
      readBootToken({
        __KANDEV_BOOT_PAYLOAD__: { runtime: { apiPrefix: "/api/v1" } },
      } as unknown as Window),
    ).toBeUndefined();
  });

  it("also surfaces the token through readBootPayload's runtime", () => {
    const win = {
      __KANDEV_BOOT_PAYLOAD__: {
        runtime: { apiPrefix: "/api/v1", webSocketPath: "/ws", csrfToken: "tok-1" },
      },
    } as unknown as Window;

    expect(readBootPayload(win).runtime?.csrfToken).toBe("tok-1");
  });
});

describe("readBootPayload", () => {
  it("returns an empty initial state when Go has not injected boot data yet", () => {
    const win = {} as Window;

    expect(readBootPayload(win)).toEqual({ initialState: {} });
  });

  it("normalizes the injected Go boot payload shape", () => {
    const win = {
      __KANDEV_BOOT_PAYLOAD__: {
        version: 1,
        route: {
          kind: "spa",
          route: "taskDetail",
          path: "/t/task-1",
          params: { taskId: "task-1" },
        },
        runtime: {
          apiPrefix: "/api/v1",
          webSocketPath: "/ws",
        },
        initialState: {
          tasks: { activeTaskId: "task-1" },
        },
      },
    } as unknown as Window;

    expect(readBootPayload(win)).toMatchObject({
      version: 1,
      route: {
        kind: "spa",
        route: "taskDetail",
        path: "/t/task-1",
        params: { taskId: "task-1" },
      },
      runtime: {
        apiPrefix: "/api/v1",
        webSocketPath: "/ws",
      },
      initialState: {
        tasks: { activeTaskId: "task-1" },
      },
    });
  });

  it("drops invalid route params instead of exposing mixed values", () => {
    const win = {
      __KANDEV_BOOT_PAYLOAD__: {
        route: {
          params: { taskId: "task-1", bad: 3 },
        },
      },
    } as unknown as Window;

    expect(readBootPayload(win).route?.params).toBeUndefined();
  });

  it("enables the runtime debug global when boot payload debug is true", () => {
    const win = {
      __KANDEV_BOOT_PAYLOAD__: {
        runtime: {
          debug: true,
        },
      },
    } as unknown as Window;

    expect(readBootPayload(win).runtime?.debug).toBe(true);
    expect(win.__KANDEV_DEBUG).toBe(true);
  });
});

describe("readBootPayload plugins", () => {
  it("parses active plugins from the boot payload", () => {
    const win = {
      __KANDEV_BOOT_PAYLOAD__: {
        plugins: [
          {
            id: "jira",
            name: "Jira",
            bundleUrl: JIRA_BUNDLE_URL,
            styleUrls: ["/api/plugins/jira/style.css"],
          },
          { id: "hello", name: "Hello", bundleUrl: "/api/plugins/hello/bundle" },
        ],
      },
    } as unknown as Window;

    expect(readBootPayload(win).plugins).toEqual([
      {
        id: "jira",
        name: "Jira",
        bundleUrl: JIRA_BUNDLE_URL,
        styleUrls: ["/api/plugins/jira/style.css"],
      },
      { id: "hello", name: "Hello", bundleUrl: "/api/plugins/hello/bundle", styleUrls: undefined },
    ]);
  });

  it("drops plugin entries missing required fields and non-string styleUrls entries", () => {
    const win = {
      __KANDEV_BOOT_PAYLOAD__: {
        plugins: [
          { id: "no-bundle-url", name: "Missing bundleUrl" },
          { id: "jira", name: "Jira", bundleUrl: JIRA_BUNDLE_URL, styleUrls: ["ok.css", 3] },
        ],
      },
    } as unknown as Window;

    expect(readBootPayload(win).plugins).toEqual([
      { id: "jira", name: "Jira", bundleUrl: JIRA_BUNDLE_URL, styleUrls: ["ok.css"] },
    ]);
  });

  it("leaves plugins undefined when the boot payload has no plugins field", () => {
    const win = { __KANDEV_BOOT_PAYLOAD__: { version: 1 } } as unknown as Window;

    expect(readBootPayload(win).plugins).toBeUndefined();
  });
});

describe("loadBootPayload", () => {
  it("uses the injected Go boot payload without fetching", async () => {
    const win = Object.assign(new Window(), {
      __KANDEV_BOOT_PAYLOAD__: { version: 1, initialState: { features: { office: true } } },
    }) as Window;
    const fetcher = vi.fn();

    await expect(loadBootPayload(win, fetcher)).resolves.toMatchObject({
      initialState: { features: { office: true } },
    });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("fetches app-state before mount when no boot payload was injected", async () => {
    const win = new Window();
    Object.defineProperty(win, "location", {
      value: { pathname: "/", search: "?workspaceId=ws-1" },
    });
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ version: 1, initialState: { workflows: { activeId: "wf-1" } } }),
    });

    await expect(loadBootPayload(win, fetcher)).resolves.toMatchObject({
      initialState: { workflows: { activeId: "wf-1" } },
    });
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/app-state?path=%2F%3FworkspaceId%3Dws-1"),
      expect.objectContaining({ cache: "no-store", credentials: "include" }),
    );
  });
});
