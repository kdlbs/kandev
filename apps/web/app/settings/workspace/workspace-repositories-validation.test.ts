import { describe, expect, it } from "vitest";
import type { RepositoryPathValidationResponse } from "@/lib/types/http";
import { isValidManualRepository } from "./workspace-repositories-validation";

describe("manual repository validation", () => {
  it("does not use discovery-root allowance to reject an existing Git repository", () => {
    const result: RepositoryPathValidationResponse = {
      path: "C:\\src\\project",
      exists: true,
      is_git: true,
      allowed: false,
    };

    expect(isValidManualRepository(result)).toBe(true);
  });

  it.each([
    { exists: false, is_git: true },
    { exists: true, is_git: false },
  ])("rejects a path when exists=$exists and is_git=$is_git", ({ exists, is_git }) => {
    const result: RepositoryPathValidationResponse = {
      path: "/src/project",
      exists,
      is_git,
      allowed: true,
    };

    expect(isValidManualRepository(result)).toBe(false);
  });
});
