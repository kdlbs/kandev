import { describe, expect, it, vi } from "vitest";
import * as submoduleReviewHelpers from "./submodule-review-helpers";

type RetryGitIndexLock = <T>(operation: () => T) => Promise<T>;

const retryGitIndexLock = (
  submoduleReviewHelpers as unknown as { retryGitIndexLock: RetryGitIndexLock }
).retryGitIndexLock;

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
