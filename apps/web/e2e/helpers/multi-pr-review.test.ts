import { describe, expect, it } from "vitest";
import { reviewRepositoryName } from "./multi-pr-review";

describe("multi-PR review repository names", () => {
  it("normalizes repository IDs while preserving distinct names", () => {
    expect(reviewRepositoryName({ repositoryId: "Repo/ID #A" })).toBe("e2e-review-repoida");
    expect(reviewRepositoryName({ repositoryId: "Repo/ID #B" })).toBe("e2e-review-repoidb");
  });
});
