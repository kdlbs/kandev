import { describe, expect, it } from "vitest";
import { isTaskIdentifierParam } from "./task-identifier-redirect";

describe("isTaskIdentifierParam", () => {
  it("accepts workspace identifiers", () => {
    expect(isTaskIdentifierParam("KAN-11")).toBe(true);
    expect(isTaskIdentifierParam("E2E-1")).toBe(true);
  });

  it("rejects UUIDs and other ids", () => {
    expect(isTaskIdentifierParam("3f2b8c1e-9a4d-4e7b-8c1d-2a5f6e7d8c9b")).toBe(false);
    expect(isTaskIdentifierParam("task-abc")).toBe(false);
    expect(isTaskIdentifierParam("task-1")).toBe(false);
  });
});
