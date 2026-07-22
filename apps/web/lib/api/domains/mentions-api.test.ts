import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { searchEntityReferences } from "./mentions-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("mentions API", () => {
  it("encodes scoped search parameters and preserves provider-neutral groups", async () => {
    const response = {
      query: "auth & oauth",
      groups: [
        {
          source: "plugin.acme_incidents",
          provider: "plugin.acme",
          kind: "incident",
          display_name: "Acme incidents",
          kind_label: "Incident",
          status: "ok",
          results: [
            {
              version: 1,
              ref: "mention:v1:plugin.acme:incident:incident-7",
              provider: "plugin.acme",
              kind: "incident",
              id: "incident-7",
              key: "INC-7",
              title: "Repair auth & oauth",
              url: "https://incidents.example.test/7",
              scope: "workspace / one",
            },
          ],
        },
        {
          source: "plugin.acme_changes",
          provider: "plugin.acme",
          kind: "change",
          display_name: "Acme changes",
          kind_label: "Change",
          status: "rate_limited",
          results: [],
        },
      ],
    };
    const controller = new AbortController();
    fetchSpy.mockResolvedValueOnce(jsonResponse(response));

    const result = await searchEntityReferences(
      {
        workspaceId: "workspace / one",
        query: "auth & oauth",
        limit: 7,
      },
      { cache: "no-store", init: { signal: controller.signal } },
    );

    expect(fetchSpy).toHaveBeenCalledWith(
      "http://api.test/api/v1/workspaces/workspace%20%2F%20one/mentions/search?q=auth+%26+oauth&limit=7",
      expect.objectContaining({ cache: "no-store", signal: controller.signal }),
    );
    expect(result).toEqual(response);
  });
});
