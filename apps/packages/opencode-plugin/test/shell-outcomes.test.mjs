import assert from "node:assert/strict";
import { test } from "node:test";

import { ShellOutcomesBehavior } from "../src/behaviors/shell-outcomes.ts";

function claim(overrides = {}) {
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
    ...overrides,
  };
}

function harness() {
  const emitted = [];
  const logs = [];
  const behavior = new ShellOutcomesBehavior();
  behavior.start({
    signal: new AbortController().signal,
    emit: (payload, nativeSessionId, nativeGeneration) => {
      emitted.push({ payload, nativeSessionId, nativeGeneration });
    },
    log: (event, fields) => logs.push({ event, fields }),
  });
  return { behavior, emitted, logs };
}

function created(shellId = "shell-1", sessionID = "session-1") {
  return {
    type: "shell.created",
    data: {
      info: {
        id: shellId,
        status: "running",
        metadata: { sessionID },
        time: { started: 1000 },
      },
    },
  };
}

function exited(shellId, status = "exited", exit) {
  return {
    type: "shell.exited",
    data: { id: shellId, status, ...(exit === undefined ? {} : { exit }) },
  };
}

test("a claimed successful shell emits one completed fact", () => {
  const { behavior, emitted } = harness();
  behavior.configure({ version: 1, data: { claims: [claim()] } });
  behavior.handleEvent(created());
  behavior.handleEvent(exited("shell-1", "exited", 0));
  assert.equal(emitted.length, 1);
  assert.deepEqual(emitted[0].payload, {
    kind: "shell.terminal",
    shellId: "shell-1",
    status: "exited",
    exitCode: 0,
    startedAt: 1000,
    completedAt: emitted[0].payload.completedAt,
  });
  assert.equal(emitted[0].nativeSessionId, "session-1");
  assert.equal(emitted[0].nativeGeneration, 1);
});

test("a nonzero exit and timeout/killed status keep their native facts", () => {
  const { behavior, emitted } = harness();
  behavior.configure({
    version: 1,
    data: {
      claims: [
        claim({ shellId: "nonzero" }),
        claim({ shellId: "timeout" }),
        claim({ shellId: "killed" }),
      ],
    },
  });
  behavior.handleEvent(created("nonzero"));
  behavior.handleEvent(created("timeout"));
  behavior.handleEvent(created("killed"));
  behavior.handleEvent(exited("nonzero", "exited", 3));
  behavior.handleEvent(exited("timeout", "timeout", undefined));
  behavior.handleEvent(exited("killed", "killed", 137));
  assert.deepEqual(
    emitted.map((item) => [item.payload.shellId, item.payload.status, item.payload.exitCode]),
    [
      ["nonzero", "exited", 3],
      ["timeout", "timeout", undefined],
      ["killed", "killed", 137],
    ],
  );
});

test("an early terminal fact is retained until its claim arrives", () => {
  const { behavior, emitted } = harness();
  behavior.handleEvent(created());
  behavior.handleEvent(exited("shell-1", "exited", 0));
  assert.equal(emitted.length, 0);
  behavior.configure({ version: 1, data: { claims: [claim()] } });
  assert.equal(emitted.length, 1);
  behavior.configure({ version: 1, data: { claims: [claim()] } });
  assert.equal(emitted.length, 1, "repeated config must not duplicate the observation");
});

test("an unclaimed foreground shell never emits background work", () => {
  const { behavior, emitted } = harness();
  behavior.handleEvent(created("foreground"));
  behavior.handleEvent(exited("foreground", "exited", 0));
  assert.equal(emitted.length, 0);
});

test("a deleted shell without an exit event remains unknown", () => {
  const { behavior, emitted } = harness();
  behavior.configure({ version: 1, data: { claims: [claim()] } });
  behavior.handleEvent(created());
  behavior.handleEvent({ type: "shell.deleted", data: { id: "shell-1" } });
  assert.equal(emitted.length, 0);
});

test("a claim for a different native session has no effect", () => {
  const { behavior, emitted, logs } = harness();
  behavior.handleEvent(created("shell-1", "session-other"));
  behavior.configure({ version: 1, data: { claims: [claim()] } });
  behavior.handleEvent(exited("shell-1", "exited", 0));
  assert.equal(emitted.length, 0);
  assert.equal(
    logs.some((entry) => entry.event === "claim_session_mismatch"),
    true,
  );
});
