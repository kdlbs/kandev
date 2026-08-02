import { describe, expect, it } from "vitest";
import { readAzureDevOpsBrowsePreferences } from "./use-azure-devops-preferences";

describe("readAzureDevOpsBrowsePreferences", () => {
  it("keeps workspace-scoped browse preferences and ignores invalid storage shapes", () => {
    const preferences = { "workspace-a": { mode: "board", board: { boardId: "board-a" } } };

    expect(readAzureDevOpsBrowsePreferences(preferences)).toEqual(preferences);
    expect(readAzureDevOpsBrowsePreferences([])).toEqual({});
    expect(readAzureDevOpsBrowsePreferences(null)).toEqual({});
  });
});
