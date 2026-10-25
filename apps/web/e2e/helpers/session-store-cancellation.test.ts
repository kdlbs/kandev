import { afterEach, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import {
  waitForActiveSessionCancellationPending,
  waitForQuickChatSessionSettled,
} from "./session-store";

afterEach(() => vi.unstubAllGlobals());

it("requires a settled cancellation revision for the addressed task session", async () => {
  let snapshot = { cancellation_pending: false, cancellation_revision: 0 };
  vi.stubGlobal("window", {
    __KANDEV_E2E_STORE__: {
      getState: () => ({ taskSessions: { items: { target: snapshot } } }),
    },
  });
  const observed: boolean[] = [];
  const page = {
    waitForFunction: async (predicate: (argument: unknown) => boolean, argument: unknown) => {
      observed.push(predicate(argument));
    },
  } as unknown as Page;

  await waitForActiveSessionCancellationPending(page, false, "target", 2);
  snapshot = { cancellation_pending: false, cancellation_revision: 1 };
  await waitForActiveSessionCancellationPending(page, false, "target", 2);
  snapshot = { cancellation_pending: true, cancellation_revision: 2 };
  await waitForActiveSessionCancellationPending(page, false, "target", 2);
  snapshot = { cancellation_pending: false, cancellation_revision: 2 };
  await waitForActiveSessionCancellationPending(page, false, "target", 2);
  await waitForActiveSessionCancellationPending(page, false, "other", 2);
  expect(observed).toEqual([false, false, false, true, false]);

  snapshot = { cancellation_pending: false, cancellation_revision: 0 };
  await waitForActiveSessionCancellationPending(page, false, "target");
  expect(observed.at(-1)).toBe(true);
});

it("requires an accepted and settled cancellation revision instead of an idle snapshot", async () => {
  let snapshot = {
    state: "WAITING_FOR_INPUT",
    cancellation_pending: false,
    cancellation_revision: 0,
    foreground_activity: null as string | null,
  };
  vi.stubGlobal("window", {
    __KANDEV_E2E_STORE__: {
      getState: () => ({ taskSessions: { items: { target: snapshot } } }),
    },
  });
  const observed: boolean[] = [];
  const page = {
    waitForFunction: async (predicate: (argument: unknown) => boolean, argument: unknown) => {
      observed.push(predicate(argument));
    },
  } as unknown as Page;

  await waitForQuickChatSessionSettled(page, "target", 20_000, 2);
  snapshot = { ...snapshot, cancellation_pending: true, cancellation_revision: 1 };
  await waitForQuickChatSessionSettled(page, "target", 20_000, 2);
  snapshot = { ...snapshot, cancellation_pending: false, cancellation_revision: 2 };
  await waitForQuickChatSessionSettled(page, "target", 20_000, 2);
  snapshot = { ...snapshot, foreground_activity: "background" };
  await waitForQuickChatSessionSettled(page, "target", 20_000, 2);
  await waitForQuickChatSessionSettled(page, "other", 20_000, 2);

  expect(observed).toEqual([false, false, true, false, false]);
});

it("preserves ordinary settle waits without requiring a cancellation", async () => {
  vi.stubGlobal("window", {
    __KANDEV_E2E_STORE__: {
      getState: () => ({
        taskSessions: {
          items: {
            target: {
              state: "WAITING_FOR_INPUT",
              cancellation_pending: false,
              cancellation_revision: 0,
              foreground_activity: null,
            },
          },
        },
      }),
    },
  });
  let settled = false;
  const page = {
    waitForFunction: async (predicate: (argument: unknown) => boolean, argument: unknown) => {
      settled = predicate(argument);
    },
  } as unknown as Page;
  await waitForQuickChatSessionSettled(page, "target");
  expect(settled).toBe(true);
});
