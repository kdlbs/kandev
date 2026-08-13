import { describe, expect, it } from "vitest";
import type { Page, Response } from "@playwright/test";
import { DWELL_CATEGORIES, dwell, waitForHttp, watchWs } from "./causal-waits";

// ---------------------------------------------------------------------------
// Doubles for the slivers of the Playwright API these primitives touch.
// ---------------------------------------------------------------------------

type FrameEvent = { payload: string | Buffer };

class FakeWebSocket {
  private readonly listeners = new Map<string, Array<(event: FrameEvent) => void>>();

  constructor(private readonly socketUrl: string) {}

  url(): string {
    return this.socketUrl;
  }

  on(event: string, listener: (event: FrameEvent) => void): void {
    const bucket = this.listeners.get(event) ?? [];
    bucket.push(listener);
    this.listeners.set(event, bucket);
  }

  /** Simulate a frame arriving in either direction. */
  emit(event: "framesent" | "framereceived", payload: unknown): void {
    const text = typeof payload === "string" ? payload : JSON.stringify(payload);
    for (const listener of this.listeners.get(event) ?? []) listener({ payload: text });
  }
}

type ResponsePredicate = (response: Response) => boolean | Promise<boolean>;

class FakePage {
  private readonly socketListeners: Array<(ws: FakeWebSocket) => void> = [];
  private readonly responseWaiters: Array<{
    predicate: ResponsePredicate;
    resolve: (response: Response) => void;
  }> = [];

  on(event: string, listener: (ws: FakeWebSocket) => void): void {
    if (event === "websocket") this.socketListeners.push(listener);
  }

  openSocket(url: string): FakeWebSocket {
    const socket = new FakeWebSocket(url);
    for (const listener of this.socketListeners) listener(socket);
    return socket;
  }

  waitForResponse(predicate: ResponsePredicate, options: { timeout: number }): Promise<Response> {
    return new Promise<Response>((resolve, reject) => {
      const waiter = { predicate, resolve };
      this.responseWaiters.push(waiter);
      setTimeout(() => {
        const index = this.responseWaiters.indexOf(waiter);
        if (index >= 0) this.responseWaiters.splice(index, 1);
        reject(new Error("Timeout exceeded while waiting for event"));
      }, options.timeout);
    });
  }

  waitedMs: number[] = [];

  waitForTimeout(ms: number): Promise<void> {
    this.waitedMs.push(ms);
    return Promise.resolve();
  }

  /** Simulate an HTTP response reaching the page. */
  async deliver(method: string, url: string, status = 200): Promise<void> {
    const response = {
      url: () => url,
      status: () => status,
      request: () => ({ method: () => method }),
    } as unknown as Response;
    for (const waiter of [...this.responseWaiters]) {
      if (!(await waiter.predicate(response))) continue;
      this.responseWaiters.splice(this.responseWaiters.indexOf(waiter), 1);
      waiter.resolve(response);
    }
  }
}

function fakePage(): { page: Page; fake: FakePage } {
  const fake = new FakePage();
  return { page: fake as unknown as Page, fake };
}

const GATEWAY = "ws://127.0.0.1:9999/ws";

// ---------------------------------------------------------------------------
// waitForHttp
// ---------------------------------------------------------------------------

describe("waitForHttp", () => {
  it("resolves on a matching method and path, ignoring the query string", async () => {
    const { page, fake } = fakePage();
    const pending = waitForHttp(page, "GET", /^\/api\/v1\/repositories\/[^/]+\/branches$/);
    await fake.deliver("GET", "http://host/api/v1/repositories/r1/branches?refresh=true");
    await expect(pending).resolves.toMatchObject({});
  });

  it("ignores a response with the right path but the wrong method", async () => {
    const { page, fake } = fakePage();
    const pending = waitForHttp(page, "POST", /\/branches$/, { timeout: 30 });
    await fake.deliver("GET", "http://host/api/v1/repositories/r1/branches");
    await expect(pending).rejects.toThrow(
      "waitForHttp: no POST response matching /\\/branches$/ within 30ms",
    );
  });

  it("applies the extra predicate on top of method and path", async () => {
    const { page, fake } = fakePage();
    const pending = waitForHttp(page, "GET", /\/branches$/, {
      predicate: (response) => response.status() === 201,
    });
    await fake.deliver("GET", "http://host/api/v1/repositories/r1/branches", 200);
    await fake.deliver("GET", "http://host/api/v1/repositories/r2/branches", 201);
    await expect(pending).resolves.toMatchObject({});
    await expect((await pending).url()).toContain("/r2/");
  });

  it("names the signal it was waiting for when it times out", async () => {
    const { page } = fakePage();
    await expect(waitForHttp(page, "PATCH", /\/office\/runs\//, { timeout: 25 })).rejects.toThrow(
      "waitForHttp: no PATCH response matching /\\/office\\/runs\\// within 25ms",
    );
  });
});

// ---------------------------------------------------------------------------
// watchWs().waitForEvent
// ---------------------------------------------------------------------------

describe("watchWs().waitForEvent", () => {
  it("resolves on a server-pushed notification with the given action", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForEvent("office.run.processed");
    socket.emit("framereceived", {
      type: "notification",
      action: "office.run.processed",
      payload: { task_id: "t1", status: "cancelled" },
    });
    await expect(pending).resolves.toMatchObject({
      action: "office.run.processed",
      payload: { task_id: "t1", status: "cancelled" },
    });
  });

  it("skips notifications that fail the `where` predicate", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForEvent("office.run.processed", {
      where: (payload) => payload.task_id === "wanted",
    });
    socket.emit("framereceived", {
      type: "notification",
      action: "office.run.processed",
      payload: { task_id: "other" },
    });
    socket.emit("framereceived", {
      type: "notification",
      action: "office.run.processed",
      payload: { task_id: "wanted" },
    });
    await expect(pending).resolves.toMatchObject({ payload: { task_id: "wanted" } });
  });

  it("ignores frames from non-gateway sockets such as terminals", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const terminal = fake.openSocket("ws://127.0.0.1:9999/api/v1/terminals/abc");
    const pending = ws.waitForEvent("office.run.processed", { timeout: 30 });
    terminal.emit("framereceived", {
      type: "notification",
      action: "office.run.processed",
      payload: {},
    });
    await expect(pending).rejects.toThrow(
      'watchWs.waitForEvent: no "office.run.processed" notification within 30ms',
    );
  });

  it("does not match a request/response frame that echoes the same action", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForEvent("task.plan.get", { timeout: 30 });
    socket.emit("framesent", { id: "r1", type: "request", action: "task.plan.get", payload: {} });
    socket.emit("framereceived", {
      id: "r1",
      type: "response",
      action: "task.plan.get",
      payload: {},
    });
    await expect(pending).rejects.toThrow(
      'watchWs.waitForEvent: no "task.plan.get" notification within 30ms',
    );
  });

  it("does not settle on a partial frame that has an action but no type", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForEvent("office.run.processed", { timeout: 30 });
    socket.emit("framereceived", { action: "office.run.processed", payload: { task_id: "t1" } });
    await expect(pending).rejects.toThrow(
      'watchWs.waitForEvent: no "office.run.processed" notification within 30ms',
    );
  });

  it("tolerates binary and malformed frames without settling", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForEvent("office.run.processed", { timeout: 30 });
    socket.emit("framereceived", "not json at all");
    await expect(pending).rejects.toThrow("watchWs.waitForEvent");
  });
});

// ---------------------------------------------------------------------------
// watchWs().waitForResponse
// ---------------------------------------------------------------------------

describe("watchWs().waitForResponse", () => {
  it("correlates the reply to the request it armed for, by frame id", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForResponse("task.plan.get");
    socket.emit("framesent", {
      id: "req-1",
      type: "request",
      action: "task.plan.get",
      payload: {},
    });
    socket.emit("framereceived", {
      id: "req-1",
      type: "response",
      action: "task.plan.get",
      payload: { content: "# Plan" },
    });
    await expect(pending).resolves.toMatchObject({ id: "req-1", payload: { content: "# Plan" } });
  });

  it("ignores a reply to a different in-flight request", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForResponse("task.plan.get", { timeout: 30 });
    socket.emit("framesent", {
      id: "req-1",
      type: "request",
      action: "task.plan.get",
      payload: {},
    });
    // Same action, different id: the gateway echoes `action` on replies, so id
    // correlation is the only thing that keeps this from being a false match.
    socket.emit("framereceived", {
      id: "other",
      type: "response",
      action: "task.plan.get",
      payload: { unrelated: true },
    });
    await expect(pending).rejects.toThrow(
      'watchWs.waitForResponse: no reply to "task.plan.get" within 30ms',
    );
  });

  it("only correlates requests sent after it was armed", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    // An earlier, already-answered plan fetch must not satisfy a later wait.
    socket.emit("framesent", { id: "old", type: "request", action: "task.plan.get", payload: {} });
    const pending = ws.waitForResponse("task.plan.get", { timeout: 30 });
    socket.emit("framereceived", {
      id: "old",
      type: "response",
      action: "task.plan.get",
      payload: { stale: true },
    });
    await expect(pending).rejects.toThrow(
      'watchWs.waitForResponse: no reply to "task.plan.get" within 30ms',
    );
  });

  it("rejects with the backend error payload when the request is refused", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    const socket = fake.openSocket(GATEWAY);
    const pending = ws.waitForResponse("task.plan.get");
    socket.emit("framesent", {
      id: "req-1",
      type: "request",
      action: "task.plan.get",
      payload: {},
    });
    socket.emit("framereceived", {
      id: "req-1",
      type: "error",
      action: "task.plan.get",
      payload: { message: "plan not found" },
    });
    await expect(pending).rejects.toThrow(
      'watchWs.waitForResponse: "task.plan.get" was rejected: {"message":"plan not found"}',
    );
  });

  it("observes sockets opened after the watcher, so it survives a reload", async () => {
    const { page, fake } = fakePage();
    const ws = watchWs(page);
    fake.openSocket(GATEWAY);
    const reconnected = fake.openSocket(GATEWAY);
    const pending = ws.waitForResponse("task.plan.get");
    reconnected.emit("framesent", {
      id: "req-2",
      type: "request",
      action: "task.plan.get",
      payload: {},
    });
    reconnected.emit("framereceived", {
      id: "req-2",
      type: "response",
      action: "task.plan.get",
      payload: { ok: true },
    });
    await expect(pending).resolves.toMatchObject({ id: "req-2" });
  });
});

// ---------------------------------------------------------------------------
// dwell
// ---------------------------------------------------------------------------

describe("dwell", () => {
  it("waits for exactly the requested wall-clock delay", async () => {
    const { page, fake } = fakePage();
    await dwell(page, 300, "library-timer", "Radix open delay publishes nothing observable");
    expect(fake.waitedMs).toEqual([300]);
  });

  it("rejects an empty reason, so the argument cannot be silenced", () => {
    const { page, fake } = fakePage();
    expect(() => dwell(page, 300, "negative-assertion", "   ")).toThrow(
      "dwell: a reason is required, and must say why no event exists to wait on",
    );
    expect(fake.waitedMs).toEqual([]);
  });

  it("rejects an unknown category at runtime, since nothing typechecks e2e", () => {
    const { page, fake } = fakePage();
    expect(() =>
      // @ts-expect-error -- the union rejects this in an editor; the runtime
      // check is what catches it in CI, where e2e is never typechecked.
      dwell(page, 300, "libary-timer", "typo in the category"),
    ).toThrow(
      'dwell: unknown category "libary-timer", expected one of negative-assertion, ' +
        "product-timer, library-timer, clock-separation, poll-interval, browser-chrome, unverified",
    );
    expect(fake.waitedMs).toEqual([]);
  });

  it("waits on a real timer when no page is in scope", async () => {
    const started = Date.now();
    await dwell(40, "poll-interval", "backend health poll; no page exists yet");
    expect(Date.now() - started).toBeGreaterThanOrEqual(35);
  });

  // The two forms are told apart by the type of argument 1, so every argument
  // shifts by one. Pin that the page-less form reads category and reason from
  // the right slots rather than treating the category as the reason.
  it("reads category and reason from the shifted slots in the page-less form", () => {
    expect(() => dwell(40, "poll-interval", "   ")).toThrow(
      "dwell: a reason is required, and must say why no event exists to wait on",
    );
    expect(() =>
      // @ts-expect-error -- exercising the runtime guard on the page-less form.
      dwell(40, "not-a-category", "a perfectly good reason"),
    ).toThrow('dwell: unknown category "not-a-category"');
  });

  it("reports a missing reason as a missing reason, not a TypeError", () => {
    const { page } = fakePage();
    expect(() =>
      // @ts-expect-error -- a JS caller can omit the 4th argument entirely.
      dwell(page, 40, "library-timer"),
    ).toThrow("dwell: a reason is required, and must say why no event exists to wait on");
  });

  it("still delegates to the page when one is given, so the wait dies with it", async () => {
    const { page, fake } = fakePage();
    await dwell(page, 250, "browser-chrome", "native dialog dismissal is not observable");
    expect(fake.waitedMs).toEqual([250]);
  });

  // Chunk 4's ratchet reports the category mix off this list, and chunk 1's
  // migrated sites are typed against it. Pin the exact set and its order so a
  // rename or silent drop is a failing test here rather than a quietly wrong
  // report there.
  it("pins the closed category set", () => {
    expect(DWELL_CATEGORIES).toEqual([
      "negative-assertion",
      "product-timer",
      "library-timer",
      "clock-separation",
      "poll-interval",
      "browser-chrome",
      "unverified",
    ]);
  });
});
