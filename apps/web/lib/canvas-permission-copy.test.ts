import { describe, expect, it } from "vitest";
import {
  buildCanvasPermissionGroups,
  canvasReleaseStatusLabel,
  canvasSourceLabel,
  formatCanvasReleaseDate,
} from "./canvas-permission-copy";

const translate = (key: string, options?: Record<string, unknown>) =>
  options?.value ? `${key}:${String(options.value)}` : key;

describe("canvas permission review copy", () => {
  it("groups readable permissions once and marks newly requested access", () => {
    const groups = buildCanvasPermissionGroups(
      {
        reads: ["tasks"],
        writes: ["messages"],
        events: ["task.updated"],
        shared_state: true,
        external_origins: ["https://example.test"],
      },
      ["api_read:tasks", "network:https://example.test"],
      translate,
    );

    expect(groups.flatMap((group) => group.rows.map((row) => row.label))).toEqual([
      "canvases:permissionReadTasks",
      "canvases:permissionWriteMessages",
      "canvases:permissionEventTaskUpdated",
      "canvases:permissionExternalOrigin",
      "canvases:permissionSharedState",
    ]);
    expect(groups[0].rows[0].isNew).toBe(true);
    expect(groups[3].rows[0].detail).toBe("https://example.test");
    expect(groups.flatMap((group) => group.rows).some((row) => row.isUnsupported)).toBe(false);
  });

  it("marks unknown permission kinds unsupported", () => {
    const groups = buildCanvasPermissionGroups(
      { reads: ["secrets"] },
      ["api_read:secrets"],
      translate,
    );
    expect(groups[0].rows[0]).toMatchObject({
      label: "canvases:unsupportedPermission:secrets",
      isNew: true,
      isUnsupported: true,
    });
  });

  it("does not approve non-origin external destinations", () => {
    const groups = buildCanvasPermissionGroups(
      {
        external_origins: [
          "https://example.test/path",
          "https://user:secret@example.test",
          "https://example.test/?scope=all",
        ],
      },
      [],
      translate,
    );

    expect(groups[0].rows).toHaveLength(3);
    expect(groups[0].rows.every((row) => row.isUnsupported)).toBe(true);
  });

  it("distinguishes active and previous releases without exposing identifiers", () => {
    expect(canvasReleaseStatusLabel("valid", "active", "active", translate)).toBe(
      "canvases:statusActive",
    );
    expect(canvasReleaseStatusLabel("valid", "previous", "active", translate)).toBe(
      "canvases:statusPrevious",
    );
    expect(canvasSourceLabel(undefined, translate)).toBe("canvases:sourceUnavailable");
  });

  it("formats valid dates and uses a safe fallback for invalid dates", () => {
    expect(formatCanvasReleaseDate("2026-01-02T03:04:05.000Z", translate)).not.toBe(
      "canvases:dateUnavailable",
    );
    expect(formatCanvasReleaseDate("not-a-date", translate)).toBe("canvases:dateUnavailable");
  });
});
