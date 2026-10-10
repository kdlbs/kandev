import { describe, expect, it } from "vitest";
import { DEMO_IDS } from "./scenario";
import { routeDemoDiscovery } from "./discovery-runtime";

const BASE = `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories`;
describe("demo repository discovery", () => {
  it.each([
    ["GET", "discovery"],
    ["GET", "discover"],
    ["POST", "discovery/refresh"],
  ])("handles %s %s with both local repositories", (method, suffix) => {
    expect(routeDemoDiscovery(`${BASE}/${suffix}`, method, new URLSearchParams())).toMatchObject({
      status: 200,
      body: {
        total: 2,
        desktop_runtime: false,
        refreshing: false,
        repositories: [
          { name: "acme-web", path: "/demo/acme-web", default_branch: "main" },
          { name: "acme-api", path: "/demo/acme-api", default_branch: "main" },
        ],
      },
    });
  });
  it("filters roots and leaves unrelated requests unsupported", () => {
    expect(
      routeDemoDiscovery(
        `${BASE}/discovery`,
        "GET",
        new URLSearchParams({ root: "/demo/acme-api" }),
      ),
    ).toMatchObject({ body: { total: 1, repositories: [{ name: "acme-api" }] } });
    expect(
      routeDemoDiscovery(`${BASE}/discovery`, "GET", new URLSearchParams({ root: "/unknown" })),
    ).toMatchObject({ body: { total: 0 } });
    expect(
      routeDemoDiscovery("/api/v1/repositories/discovery/roots", "GET", new URLSearchParams()),
    ).toMatchObject({ body: { roots: [{ state: "connected", path: "/demo" }] } });
    expect(routeDemoDiscovery("/api/v1/unhandled", "GET", new URLSearchParams())).toBeNull();
  });
});
