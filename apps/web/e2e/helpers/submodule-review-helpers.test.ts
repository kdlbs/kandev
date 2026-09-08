import { describe, expect, it, vi } from "vitest";
import { retryGitIndexLock } from "../tests/review/git-retry";

describe("retryGitIndexLock", () => {
  it("retries a transient Git index lock before returning the operation result", async () => {
    const operation = vi
      .fn<() => string>()
      .mockImplementationOnce(() => {
        throw new Error("fatal: Unable to create index.lock: File exists");
      })
      .mockReturnValueOnce("complete");

    await expect(retryGitIndexLock(operation)).resolves.toBe("complete");
    expect(operation).toHaveBeenCalledTimes(2);
  });
});
