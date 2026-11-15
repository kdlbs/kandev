import assert from "node:assert/strict";
import { test } from "node:test";

import { productionBehaviors, behaviorRefs } from "../src/behaviors/registry.ts";
import { SHELL_OUTCOMES_BEHAVIOR, SHELL_OUTCOMES_VERSION } from "../src/protocol.ts";

test("production registry exposes only the approved shell-outcomes behavior", () => {
  const behaviors = productionBehaviors();
  assert.deepEqual(behaviorRefs(behaviors), [
    { id: SHELL_OUTCOMES_BEHAVIOR, version: SHELL_OUTCOMES_VERSION },
  ]);
});

test("a second compiled behavior needs no core rewrite", () => {
  const testBehavior = {
    id: "test-behavior",
    version: 7,
    eventTypes: ["test.event"],
    start() {},
    configure() {},
    observationKey() {
      return "test-key";
    },
  };
  const refs = behaviorRefs([...productionBehaviors(), testBehavior]);
  assert.deepEqual(refs, [
    { id: SHELL_OUTCOMES_BEHAVIOR, version: 1 },
    { id: "test-behavior", version: 7 },
  ]);
});

test("each production behavior declares required lifecycle methods", () => {
  for (const behavior of productionBehaviors()) {
    assert.equal(typeof behavior.id, "string");
    assert.ok(behavior.version > 0);
    assert.ok(Array.isArray(behavior.eventTypes));
    assert.equal(typeof behavior.start, "function");
    assert.equal(typeof behavior.configure, "function");
  }
});
