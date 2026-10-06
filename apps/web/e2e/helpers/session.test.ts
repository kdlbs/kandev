import type { Page } from "@playwright/test";
import { afterEach, expect, it, vi } from "vitest";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { seedIdleSession, waitForAgentMessage } from "./session";

vi.mock("../pages/session-page", () => ({
  SessionPage: class {
    waitForLoad() {
      return Promise.resolve();
    }
    waitForChatIdle() {
      return Promise.resolve();
    }
    composerReady() {
      return Promise.resolve();
    }
  },
}));

afterEach(() => vi.useRealTimers());

it("waits for the expected agent message rather than any earlier transcript activity", async () => {
  const snapshots = [
    [],
    [{ author_type: "user", content: "started" }],
    [{ author_type: "agent", content: "started" }],
  ];
  let reads = 0;
  const apiClient = {
    listSessionMessages: async () => ({
      messages: snapshots[Math.min(reads++, snapshots.length - 1)]!,
    }),
  } as unknown as ApiClient;

  await waitForAgentMessage(apiClient, "session-1", "started", 2_000);

  expect(reads).toBe(3);
});

it("does not expose an idle fixture before its opening response and settled state", async () => {
  vi.useFakeTimers();
  let hasOpeningResponse = false;
  let state = "WAITING_FOR_INPUT";
  const apiClient = {
    createTaskWithAgent: vi.fn().mockResolvedValue({ id: "task", session_id: "session" }),
    listSessionMessages: vi.fn(async () => ({
      messages: hasOpeningResponse
        ? [{ author_type: "agent", content: "This is a simple mock response for e2e testing." }]
        : [],
    })),
    listTaskSessions: vi.fn(async () => ({ sessions: [{ id: "session", state }] })),
  } as unknown as ApiClient;
  const page = { goto: vi.fn().mockResolvedValue(undefined) } as unknown as Page;
  const seed = {} as SeedData;
  let returned = false;
  const ready = seedIdleSession(page, apiClient, seed, "Opening turn").then(() => {
    returned = true;
  });

  await vi.advanceTimersByTimeAsync(1_000);
  expect(returned).toBe(false);

  hasOpeningResponse = true;
  state = "RUNNING";
  await vi.advanceTimersByTimeAsync(1_000);
  expect(returned).toBe(false);

  state = "WAITING_FOR_INPUT";
  await vi.advanceTimersByTimeAsync(250);
  await ready;
  expect(returned).toBe(true);
});
