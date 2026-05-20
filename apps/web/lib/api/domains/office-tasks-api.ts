// Office tasks list API: split from office-extended-api.ts so the
// list/filter/sort/paginate surface (Stream E of office optimization)
// can grow without pushing the parent file past the eslint max-lines
// budget.
import { fetchJson, type ApiRequestOptions } from "../client";
import type { OfficeTask } from "@/lib/state/slices/office/types";

const BASE = "/api/v1/office";

export type ListTasksParams = {
  // Server-side filters / sort / pagination. When omitted the backend
  // falls back to the legacy unbounded listing.
  status?: string[];
  priority?: string[];
  assignee?: string;
  project?: string;
  // Allowed sort values: "updated_at" | "created_at" | "priority". Other
  // values are rejected by the backend allow-list.
  sort?: "updated_at" | "created_at" | "priority";
  order?: "asc" | "desc";
  limit?: number;
  cursor?: string;
  cursor_id?: string;
  // When true, kandev-managed system tasks (today: standing
  // coordination; future: routine-fired) are returned alongside user
  // tasks. Default false hides them from the Tasks UI.
  include_system?: boolean;
};

export type ListTasksResponse = {
  tasks: OfficeTask[];
  next_cursor?: string;
  next_id?: string;
};

function buildTaskListQuery(params: ListTasksParams | undefined): string {
  if (!params) return "";
  const usp = new URLSearchParams();
  for (const v of params.status ?? []) usp.append("status", v);
  for (const v of params.priority ?? []) usp.append("priority", v);
  if (params.assignee) usp.set("assignee", params.assignee);
  if (params.project) usp.set("project", params.project);
  if (params.sort) usp.set("sort", params.sort);
  if (params.order) usp.set("order", params.order);
  if (params.limit) usp.set("limit", String(params.limit));
  if (params.cursor) usp.set("cursor", params.cursor);
  if (params.cursor_id) usp.set("cursor_id", params.cursor_id);
  if (params.include_system) usp.set("include_system", "true");
  const s = usp.toString();
  return s ? `?${s}` : "";
}

// Known ApiRequestOptions keys — used to disambiguate the legacy
// `listTasks(workspaceId, options)` call signature from the new
// `listTasks(workspaceId, params, options?)` form.
const API_OPTIONS_KEYS = ["baseUrl", "cache", "init"] as const;

function looksLikeApiOptions(v: object): boolean {
  for (const k of API_OPTIONS_KEYS) {
    if (k in v) return true;
  }
  return false;
}

export function listTasks(
  workspaceId: string,
  paramsOrOptions?: ListTasksParams | ApiRequestOptions,
  maybeOptions?: ApiRequestOptions,
) {
  let params: ListTasksParams | undefined;
  let options: ApiRequestOptions | undefined;
  if (paramsOrOptions && looksLikeApiOptions(paramsOrOptions as object)) {
    options = paramsOrOptions as ApiRequestOptions;
  } else {
    params = paramsOrOptions as ListTasksParams | undefined;
    options = maybeOptions;
  }
  return fetchJson<ListTasksResponse>(
    `${BASE}/workspaces/${workspaceId}/tasks${buildTaskListQuery(params)}`,
    options,
  );
}
