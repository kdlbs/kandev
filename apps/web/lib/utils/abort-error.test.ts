import { describe, expect, it } from "vitest";
import { createAbortError, isAbortError } from "./abort-error";

describe("abort error helpers", () => {
  it("creates and recognizes the shared abort marker", () => {
    const error = createAbortError("superseded");

    expect(error).toBeInstanceOf(Error);
    expect(error.message).toBe("superseded");
    expect(error.name).toBe("AbortError");
    expect(isAbortError(error)).toBe(true);
  });

  it("recognizes cross-realm abort-shaped errors", () => {
    expect(isAbortError({ name: "AbortError" })).toBe(true);
    expect(isAbortError(new Error("ordinary"))).toBe(false);
    expect(isAbortError(null)).toBe(false);
  });
});
