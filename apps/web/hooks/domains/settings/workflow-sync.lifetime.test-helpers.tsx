import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { ApiRequestOptions } from "@/lib/api/client";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";

export function Providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <ToastProvider>{children}</ToastProvider>
    </StateProvider>
  );
}

export function config(workspace = "A", overrides: Partial<WorkflowSyncConfig> = {}) {
  return {
    workspace_id: workspace,
    provider: "github" as const,
    repo_owner: "team",
    repo_name: workspace,
    project_path: "",
    branch: "main",
    path: ".kandev/workflows",
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

export function deferred<T = unknown>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

export function transport() {
  const configs = new Map<string, WorkflowSyncConfig | undefined>([
    ["A", config("A")],
    ["B", config("B")],
  ]);
  const requests: { workspace: string; method: string; path: string; body?: string }[] = [];
  const holds: { method: string; path: string; response: ReturnType<typeof deferred> }[] = [];
  const pending: ReturnType<typeof deferred>[] = [];
  const hold = (method: string, path = "/api/v1/workflow-sync/config") => {
    const response = deferred();
    pending.push(response);
    holds.push({ method, path, response });
    return response;
  };
  const fetch = (path: string, options?: ApiRequestOptions) => {
    const url = new URL(path, "http://localhost");
    if (url.pathname === "/api/v1/system/logs/frontend-errors") return Promise.resolve();
    const method = options?.init?.method ?? "GET";
    const workspace = url.searchParams.get("workspace_id") ?? "";
    requests.push({ workspace, method, path: url.pathname, body: options?.init?.body as string });
    const held = holds.findIndex((entry) => entry.method === method && entry.path === url.pathname);
    if (held >= 0) return holds.splice(held, 1)[0].response.promise;
    if (method === "GET") return Promise.resolve(configs.get(workspace));
    if (method === "DELETE") return Promise.resolve({ deleted: true });
    if (url.pathname.endsWith("/sync")) {
      return Promise.resolve({ config: configs.get(workspace), result: { unchanged: false } });
    }
    return Promise.resolve(config(workspace, JSON.parse(options?.init?.body as string)));
  };
  const drain = () => pending.forEach((response) => response.resolve(undefined));
  return { configs, requests, hold, fetch, drain };
}
