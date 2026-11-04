import assert from "node:assert/strict";
import { test } from "node:test";

import { startCore } from "../src/core/core.ts";
import { ShellOutcomesBehavior } from "../src/behaviors/shell-outcomes.ts";
import { makeContext, makeEventSource, startFakeBridge } from "./helpers/bridge.mjs";

function claim() {
  return {
    shellId: "shell-1",
    nativeSessionId: "session-1",
    nativeGeneration: 1,
    workId: "shell-1",
    runId: "shell-1",
    sourceCallId: "call-1",
    title: "run tests",
    promptGeneration: 3,
    submissionId: "submission-1",
  };
}

test("core starts the subscription before hello and delivers a claimed fact", async () => {
  const bridge = await startFakeBridge({ claims: [claim()] });
  const events = makeEventSource();
  const context = makeContext({ bootstrapFile: bridge.bootstrapFile, eventSource: events.stream });
  const cleanup = await startCore(context, [new ShellOutcomesBehavior()]);
  try {
    assert.equal(bridge.state.hellos.length, 1);
    await until(() => bridge.state.configPolls.length > 0 && bridge.state.events.length === 0);
    events.push({
      type: "shell.created",
      data: {
        info: { id: "shell-1", metadata: { sessionID: "session-1" }, time: { started: 1000 } },
      },
    });
    events.push({ type: "shell.exited", data: { id: "shell-1", status: "exited", exit: 0 } });
    await until(() => bridge.state.events.length === 1);
    const observation = bridge.state.events[0];
    assert.equal(observation.behaviorId, "shell-outcomes");
    assert.equal(observation.nativeSessionId, "session-1");
    assert.equal(observation.nativeGeneration, 1);
    assert.equal(observation.payload.status, "exited");
    assert.equal(observation.payload.exitCode, 0);
  } finally {
    await cleanup();
    await bridge.close();
  }
});

test("cleanup stops the subscription and no later fact is delivered", async () => {
  const bridge = await startFakeBridge({ claims: [claim()] });
  const events = makeEventSource();
  const context = makeContext({ bootstrapFile: bridge.bootstrapFile, eventSource: events.stream });
  const cleanup = await startCore(context, [new ShellOutcomesBehavior()]);
  await until(() => bridge.state.configPolls.length > 0);
  await cleanup();
  events.push({
    type: "shell.created",
    data: {
      info: { id: "shell-1", metadata: { sessionID: "session-1" }, time: { started: 1000 } },
    },
  });
  events.push({ type: "shell.exited", data: { id: "shell-1", status: "exited", exit: 0 } });
  await new Promise((resolve) => setTimeout(resolve, 100));
  assert.equal(bridge.state.events.length, 0);
  await bridge.close();
});

test("only negotiated behaviors start", async () => {
  const bridge = await startFakeBridge({ behaviorVersions: {} });
  const events = makeEventSource();
  const context = makeContext({ bootstrapFile: bridge.bootstrapFile, eventSource: events.stream });
  let started = false;
  const testBehavior = {
    id: "test-behavior",
    version: 1,
    eventTypes: ["test.event"],
    start() {
      started = true;
    },
    configure() {},
  };
  const cleanup = await startCore(context, [testBehavior]);
  try {
    assert.equal(started, false);
  } finally {
    await cleanup();
    await bridge.close();
  }
});

test("a malformed bootstrap file fails before any request", async () => {
  const bridge = await startFakeBridge();
  const events = makeEventSource();
  const context = makeContext({ bootstrapFile: bridge.bootstrapFile, eventSource: events.stream });
  context.options = { bootstrapFile: bridge.bootstrapFile };
  const { chmod } = await import("node:fs/promises");
  await chmod(bridge.bootstrapFile, 0o644);
  await assert.rejects(() => startCore(context, [new ShellOutcomesBehavior()]), /bootstrap/);
  await bridge.close();
});

async function until(predicate, timeoutMs = 3000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("condition was not met before the timeout");
}

test("a compiled non-shell behavior negotiates and delivers without core shell branches", async () => {
  const bridge = await startFakeBridge({
    behaviorVersions: { "shell-outcomes": 1, "test-behavior": 7 },
    behaviorConfigs: { "test-behavior": { resource: "r1" } },
  });
  const events = makeEventSource();
  const context = makeContext({ bootstrapFile: bridge.bootstrapFile, eventSource: events.stream });
  class SecondBehavior {
    id = "test-behavior";
    version = 7;
    eventTypes = ["test.event"];
    context;
    config;
    start(context) {
      this.context = context;
    }
    configure(config) {
      this.config = config;
    }
    observationKey(payload) {
      return `test:${payload.resource}`;
    }
    handleEvent(event) {
      if (event.type !== "test.event") return;
      if (event.data.resource !== this.config.data.resource) return;
      this.context.emit(
        { kind: "test.completed", resource: event.data.resource },
        event.data.nativeSessionId,
        event.data.nativeGeneration,
      );
    }
    dispose() {}
  }
  const cleanup = await startCore(context, [new ShellOutcomesBehavior(), new SecondBehavior()]);
  try {
    await until(() => bridge.state.configPolls.length > 0);
    events.push({
      type: "test.event",
      data: { resource: "r1", nativeSessionId: "native-1", nativeGeneration: 2 },
    });
    await until(() => bridge.state.events.length === 1);
    const observation = bridge.state.events[0];
    assert.equal(observation.behaviorId, "test-behavior");
    assert.equal(observation.nativeSessionId, "native-1");
    assert.equal(observation.nativeGeneration, 2);
    assert.deepEqual(observation.payload, { kind: "test.completed", resource: "r1" });
  } finally {
    await cleanup();
    await bridge.close();
  }
});
