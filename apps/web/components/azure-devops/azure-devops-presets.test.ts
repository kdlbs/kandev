import { describe, expect, it } from "vitest";
import {
  AZURE_PULL_REQUEST_PRESETS,
  AZURE_WORK_ITEM_PRESETS,
  presetsForKind,
} from "./azure-devops-presets";

describe("Azure DevOps presets", () => {
  it("uses Azure WIQL identity macros for personal work-item presets", () => {
    expect(
      AZURE_WORK_ITEM_PRESETS.find((preset) => preset.value === "assigned")?.filters.wiql,
    ).toContain("[System.AssignedTo] = @Me");
    expect(
      AZURE_WORK_ITEM_PRESETS.find((preset) => preset.value === "created")?.filters.wiql,
    ).toContain("[System.CreatedBy] = @Me");
  });

  it("uses the backend @me identity sentinel for personal pull-request presets", () => {
    expect(
      AZURE_PULL_REQUEST_PRESETS.find((preset) => preset.value === "review-requested")?.filters,
    ).toMatchObject({ status: "active", reviewer: "@me" });
    expect(
      AZURE_PULL_REQUEST_PRESETS.find((preset) => preset.value === "created")?.filters,
    ).toMatchObject({ status: "active", creator: "@me" });
  });

  it("maps workspace-resolved Azure queries into browse shortcuts", () => {
    const presets = presetsForKind("pull_request", {
      workItems: [],
      pullRequests: [
        {
          id: "team-review",
          label: "Team review",
          group: "created",
          filters: { status: "active", reviewer: "team-id", creator: "@me" },
        },
      ],
    });

    expect(presets).toHaveLength(1);
    expect(presets[0]).toMatchObject({
      value: "team-review",
      label: "Team review",
      group: "created",
      filters: { status: "active", reviewer: "team-id", creator: "@me" },
    });
  });
});
