import { isRangeKey } from "@/app/stats/stats-utils";
import type { SpaRoute } from "./spa-routes";

const STATIC_TOP_LEVEL_ROUTES: Record<string, SpaRoute> = {
  "/tasks": { kind: "tasks" },
  "/threads": { kind: "threads" },
  "/github": { kind: "github" },
  "/gitlab": { kind: "gitlab" },
  "/azure-devops": { kind: "azure-devops" },
  "/jira": { kind: "jira" },
  "/linear": { kind: "linear" },
  "/login": { kind: "login" },
  "/setup": { kind: "setup" },
};

export function resolveTopLevelRoute(
  normalized: string,
  searchParams: URLSearchParams,
): SpaRoute | null {
  const staticRoute = STATIC_TOP_LEVEL_ROUTES[normalized];
  if (staticRoute) return staticRoute;
  if (normalized === "/invite")
    return { kind: "invite", token: searchParams.get("token") ?? undefined };
  if (normalized === "/stats") return rangedRoute("stats", searchParams);
  if (normalized === "/stats/token-usage") return rangedRoute("tokenUsage", searchParams);
  return null;
}

function rangedRoute(kind: "stats" | "tokenUsage", searchParams: URLSearchParams): SpaRoute {
  const range = searchParams.get("range");
  return { kind, range: range && isRangeKey(range) ? range : undefined };
}
