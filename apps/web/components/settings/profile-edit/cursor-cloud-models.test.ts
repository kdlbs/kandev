import { describe, expect, it } from "vitest";
import type { CursorCloudConfigResponse } from "@/lib/api/domains/cursor-cloud-api";
import type { Executor } from "@/lib/types/http";
import { cursorCloudModelEntries, cursorCloudProfiles } from "./cursor-cloud-models";

describe("Cursor Cloud profile models", () => {
  it("loads catalogs only from saved cloud executor profiles with both references", () => {
    const executors = [
      {
        id: "cloud",
        type: "cursor_cloud",
        profiles: [
          {
            id: "ready",
            config: {
              cursor_cloud_api_key_secret_id: "secret-1",
              cursor_cloud_callback_url: "https://example.test/api/v1/managed-agent-mcp",
            },
          },
          { id: "incomplete", config: { cursor_cloud_api_key_secret_id: "secret-2" } },
        ],
      },
      { id: "local", type: "local", profiles: [{ id: "ignored", config: {} }] },
    ] as Executor[];

    expect(cursorCloudProfiles(executors)).toEqual([
      {
        secretId: "secret-1",
        callbackUrl: "https://example.test/api/v1/managed-agent-mcp",
      },
    ]);
  });

  it("merges duplicate model IDs without changing the provider identity", () => {
    const catalogs: CursorCloudConfigResponse[] = [
      { connected: true, models: [{ id: "composer-2", displayName: "Composer 2" }] },
      { connected: true, models: [{ id: "composer-2", displayName: "Composer 2 duplicate" }] },
    ];

    expect(cursorCloudModelEntries(catalogs)).toEqual([
      {
        id: "composer-2",
        name: "Composer 2",
        description: undefined,
        source: "static",
      },
    ]);
  });
});
