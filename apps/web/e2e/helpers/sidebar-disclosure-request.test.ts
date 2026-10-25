import type { Locator, Page, Route } from "@playwright/test";
import { expect, it, vi } from "vitest";
import { heldDisclosure } from "./sidebar-disclosure-request";

function fixture() {
  let reject!: (error: Error) => void;
  let resolve!: () => void;
  const response = new Promise<void>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  let handler!: (route: Route) => Promise<void>;
  let routeCompletion!: Promise<void>;
  const route = {
    request: () => ({ postDataJSON: () => ({ collapsed_group_keys: ["repo-a"] }) }),
    continue: vi.fn().mockResolvedValue(undefined),
  } as unknown as Route;
  const unroute = vi.fn().mockResolvedValue(undefined);
  const page = {
    route: vi.fn(async (_pattern: string, callback: typeof handler) => {
      handler = callback;
    }),
    waitForResponse: () => response,
    unrouteAll: unroute,
  } as unknown as Page;
  const header = {
    getAttribute: async (name: string) => (name === "aria-expanded" ? "true" : "repo-a"),
    click: async () => {
      routeCompletion = handler(route);
    },
  } as unknown as Locator;
  return { page, header, resolve, reject, unroute, drainRoute: () => routeCompletion };
}

it("preserves the assertion failure when the response and route cleanup also fail", async () => {
  const f = fixture();
  const assertion = new Error("rows disappeared");
  f.unroute.mockRejectedValueOnce(new Error("cleanup failed"));
  await expect(
    heldDisclosure(f.page, f.header, async () => {
      f.reject(new Error("response timeout"));
      throw assertion;
    }),
  ).rejects.toBe(assertion);
  expect(f.unroute).toHaveBeenCalledWith({ behavior: "wait" });
  await f.drainRoute();
});

it("cleans up after an assertion failure without waiting for a missing response", async () => {
  const f = fixture();
  const assertion = new Error("headings disappeared");
  await expect(
    heldDisclosure(f.page, f.header, async () => {
      throw assertion;
    }),
  ).rejects.toBe(assertion);
  expect(f.unroute).toHaveBeenCalledOnce();
  await f.drainRoute();
  f.reject(new Error("late response timeout"));
  await Promise.resolve();
});

it("preserves a response failure and still removes the route", async () => {
  const f = fixture();
  const failure = new Error("request failed");
  await expect(heldDisclosure(f.page, f.header, async () => f.reject(failure))).rejects.toBe(
    failure,
  );
  expect(f.unroute).toHaveBeenCalledOnce();
  await f.drainRoute();
});

it("waits for the successful response before removing the route", async () => {
  const f = fixture();
  await heldDisclosure(f.page, f.header, async () => f.resolve());
  expect(f.unroute).toHaveBeenCalledOnce();
  await f.drainRoute();
});
