import { describe, expect, it } from "vitest";
import { readBootPayload } from "./boot-payload";

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
});
