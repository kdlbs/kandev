import { describe, expect, it } from "vitest";
import { normalizeAzureWatchInput } from "./use-azure-devops-watches";

describe("normalizeAzureWatchInput", () => {
  it("keeps provider filters separate and clamps polling/in-flight values", () => {
    const normalized = normalizeAzureWatchInput({
      projectId: "project-1",
      wiql: "SELECT [System.Id] FROM WorkItems",
      repositoryId: "kandev-repo",
      baseBranch: "main",
      pollIntervalSeconds: 10,
      maxInflightTasks: 0,
    });
    expect(normalized.pollIntervalSeconds).toBe(60);
    expect(normalized.maxInflightTasks).toBeUndefined();
    expect(normalized.repositoryId).toBe("kandev-repo");
  });
});
