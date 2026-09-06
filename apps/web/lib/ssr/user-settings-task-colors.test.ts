import { describe, expect, it } from "vitest";
import { mapUserSettingsData, mapUserSettingsResponse } from "./user-settings";
import { workspaceId as toWorkspaceId } from "@/lib/types/ids";
import type { SidebarTaskColorsApi } from "@/lib/types/http-user-settings";

const UPDATED_AT = "2026-01-01T00:00:00Z";
const DEFAULT_USER_ID = "default-user";

describe("manual task-color settings", () => {
  it("maps valid colors and preserves clear tombstones", () => {
    const result = mapUserSettingsResponse({
      settings: {
        user_id: DEFAULT_USER_ID,
        workspace_id: toWorkspaceId(""),
        repository_ids: [],
        sidebar_task_colors: {
          "task-red": "red",
          "task-cleared": null,
          "task-invalid": "gray",
        } as unknown as SidebarTaskColorsApi,
        updated_at: UPDATED_AT,
      },
    });

    expect(result.sidebarTaskColors).toEqual({
      "task-red": "red",
      "task-cleared": null,
    });
  });

  it("keeps the current map when a partial settings update omits colors", () => {
    const current = {
      ...mapUserSettingsResponse(null),
      sidebarTaskColors: { "task-red": "red" as const },
    };
    const result = mapUserSettingsData({}, current);
    expect(result.sidebarTaskColors).toEqual({ "task-red": "red" });
  });
});
