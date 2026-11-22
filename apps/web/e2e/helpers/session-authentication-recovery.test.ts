import { describe, expect, it } from "vitest";
import { assertWhitespaceFreeFixtureCommandPath } from "./session-authentication-recovery";

describe("authentication recovery fixture command path", () => {
  it("accepts paths that resolve as one command argument", () => {
    expect(() =>
      assertWhitespaceFreeFixtureCommandPath("/tmp/kandev-e2e/cursor-fixture.mjs"),
    ).not.toThrow();
  });

  it("rejects whitespace because agent commands are split into fields", () => {
    expect(() =>
      assertWhitespaceFreeFixtureCommandPath("/tmp/kandev e2e/cursor-fixture.mjs"),
    ).toThrow("E2E backend temp paths must not contain whitespace");
  });
});
