import type { RepositoryDiscoveryResponse } from "@/lib/types/http";
import { DEMO_IDS, demoApiRepository, demoRepository } from "./scenario";
import type { DemoHttpResponse } from "./protocol";

export function routeDemoDiscovery(
  path: string,
  method: string,
  params: URLSearchParams,
): DemoHttpResponse | null {
  if (path === "/api/v1/repositories/discovery/roots" && method === "GET") {
    return json({
      roots: [{ id: "demo-root", path: "/demo", display_path: "/demo", state: "connected" }],
    });
  }
  const base = `/api/v1/workspaces/${DEMO_IDS.workspace}/repositories`;
  if (
    !(
      (method === "GET" && (path === `${base}/discovery` || path === `${base}/discover`)) ||
      (method === "POST" && path === `${base}/discovery/refresh`)
    )
  )
    return null;
  const root = params.get("root");
  const repositories = [demoRepository, demoApiRepository]
    .filter((repo) => !root || root === "/demo" || root === repo.local_path)
    .map((repo) => ({
      path: repo.local_path!,
      name: repo.name,
      default_branch: repo.default_branch,
    }));
  const body: RepositoryDiscoveryResponse = {
    roots: ["/demo"],
    repositories,
    total: repositories.length,
    desktop_runtime: false,
    cached: true,
    refreshing: false,
    scan_time: new Date().toISOString(),
  };
  return json(body);
}

function json(body: unknown): DemoHttpResponse {
  return { status: 200, headers: { "Content-Type": "application/json" }, body };
}
