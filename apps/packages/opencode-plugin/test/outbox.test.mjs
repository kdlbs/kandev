import assert from "node:assert/strict";
import { test } from "node:test";

import { Outbox } from "../src/core/outbox.ts";

function observation(id) {
  return {
    eventId: id,
    behaviorId: "shell-outcomes",
    behaviorVersion: 1,
    sequence: 1,
    nativeSessionId: "session-1",
    nativeGeneration: 1,
    payload: { kind: "shell.terminal", shellId: id, status: "exited", exitCode: 0 },
  };
}

function fakeClock() {
  const state = { now: 0, pending: [] };
  return {
    now: () => state.now,
    pending: state.pending,
    sleep(ms, signal) {
      return new Promise((resolve) => {
        const entry = {
          ms,
          done: false,
          release() {
            if (entry.done) return;
            entry.done = true;
            signal.removeEventListener("abort", onAbort);
            state.now += ms;
            resolve();
          },
        };
        const onAbort = () => entry.release();
        state.pending.push(entry);
        signal.addEventListener("abort", onAbort, { once: true });
      });
    },
    releaseNext() {
      const entry = state.pending.shift();
      if (!entry) throw new Error("no pending backoff");
      entry.release();
      return entry.ms;
    },
  };
}

async function waitUntil(predicate, label, timeoutMs = 1000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 2));
  }
  throw new Error(`timed out waiting for ${label}`);
}

test("outbox awaits exponential backoff before retrying a failed batch", async () => {
  const clock = fakeClock();
  const logs = [];
  let sends = 0;
  const outbox = new Outbox({
    signal: new AbortController().signal,
    now: clock.now,
    sleep: clock.sleep,
    random: () => 0,
    send: async () => {
      sends += 1;
      throw new Error("host unavailable");
    },
    log: (event) => logs.push(event),
  });
  outbox.enqueue(observation("first"));
  await waitUntil(() => sends === 1 && clock.pending.length === 1, "first failed send and backoff");
  assert.equal(clock.pending[0].ms, 250);

  // A fast failure must not spin into another attempt, and a new enqueue must
  // not bypass the failed batch's delay.
  await new Promise((resolve) => setTimeout(resolve, 30));
  assert.equal(sends, 1);
  outbox.enqueue(observation("second"));
  await new Promise((resolve) => setTimeout(resolve, 30));
  assert.equal(sends, 1);

  clock.releaseNext();
  await waitUntil(
    () => sends === 2 && clock.pending.length === 1,
    "second failed send and backoff",
  );
  assert.equal(clock.pending[0].ms, 500);

  clock.releaseNext();
  await waitUntil(() => sends === 3 && clock.pending.length === 1, "third failed send and backoff");
  assert.equal(clock.pending[0].ms, 1000);
  assert.equal(logs.includes("observation_retry_exhausted"), false);

  await outbox.close();
});

test("outbox drains the queue after a bounded failure recovers", async () => {
  const clock = fakeClock();
  const accepted = [];
  let sends = 0;
  const outbox = new Outbox({
    signal: new AbortController().signal,
    now: clock.now,
    sleep: clock.sleep,
    random: () => 0,
    send: async (events) => {
      sends += 1;
      if (sends === 1) throw new Error("transient");
      accepted.push(...events.map((event) => event.eventId));
      return { accepted: events.map((event) => event.eventId), rejected: [] };
    },
    log: () => undefined,
  });
  outbox.enqueue(observation("first"));
  outbox.enqueue(observation("second"));
  await waitUntil(() => sends === 1 && clock.pending.length === 1, "first failure");
  clock.releaseNext();
  await waitUntil(() => accepted.length === 2, "successful retry");
  await outbox.close();
  assert.deepEqual(accepted.sort(), ["first", "second"]);
});

test("outbox cancellation stops a pending backoff without another send", async () => {
  const clock = fakeClock();
  let sends = 0;
  const outbox = new Outbox({
    signal: new AbortController().signal,
    now: clock.now,
    sleep: clock.sleep,
    random: () => 0,
    send: async () => {
      sends += 1;
      throw new Error("transient");
    },
    log: () => undefined,
  });
  outbox.enqueue(observation("first"));
  await waitUntil(() => sends === 1 && clock.pending.length === 1, "first failure");
  await outbox.close();
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(sends, 1);
});

test("outbox external abort stops retries", async () => {
  const clock = fakeClock();
  const controller = new AbortController();
  let sends = 0;
  const outbox = new Outbox({
    signal: controller.signal,
    now: clock.now,
    sleep: clock.sleep,
    random: () => 0,
    send: async () => {
      sends += 1;
      throw new Error("transient");
    },
    log: () => undefined,
  });
  outbox.enqueue(observation("first"));
  await waitUntil(() => sends === 1 && clock.pending.length === 1, "first failure");
  controller.abort();
  await outbox.close();
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(sends, 1);
});
