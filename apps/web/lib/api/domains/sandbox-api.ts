import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  SandboxAvailability,
  SandboxBinding,
  SandboxBindingScope,
  SandboxPolicy,
  SandboxProvider,
  SandboxRevision,
  SandboxSelectionContext,
  SandboxSelectionPreview,
} from "@/lib/types/sandbox";

const BASE = "/api/v1/sandbox";
// i18n-exempt: Private control-flow error. Callers display a localized loading failure.
const INVALID_CATALOG = Symbol("invalid sandbox catalog pagination");

export async function checkSandboxAvailability(
  context: SandboxSelectionContext,
  options?: ApiRequestOptions,
): Promise<SandboxAvailability> {
  return fetchJson<SandboxAvailability>(`${BASE}/availability`, {
    ...options,
    cache: "no-store",
    init: { ...options?.init, method: "POST", body: JSON.stringify(context) },
  });
}

export async function fetchSandboxPolicies(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<SandboxRevision[]> {
  const result: SandboxRevision[] = [];
  const seen = new Set<string>();
  let after = "";
  do {
    const query = new URLSearchParams({ workspace_id: workspaceId, after });
    const page = await fetchJson<{ policies: SandboxRevision[]; next_cursor?: string }>(
      `${BASE}/policies?${query}`,
      { ...options, cache: "no-store" },
    );
    result.push(...page.policies);
    after = page.next_cursor ?? "";
    if (after && (seen.has(after) || seen.size >= 127)) throw INVALID_CATALOG;
    seen.add(after);
  } while (after);
  return result;
}

export function fetchSandboxProviders(options?: ApiRequestOptions) {
  return fetchJson<{ providers: SandboxProvider[] }>(`${BASE}/providers`, {
    ...options,
    cache: "no-store",
  });
}

export function fetchSandboxBinding(
  scope: SandboxBindingScope,
  targetId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxBinding>(`${BASE}/bindings/${scope}/${encodeURIComponent(targetId)}`, {
    ...options,
    cache: "no-store",
  });
}

export function fetchSandboxRevision(id: string, options?: ApiRequestOptions) {
  return fetchJson<SandboxRevision>(`${BASE}/revisions/${encodeURIComponent(id)}`, {
    ...options,
    cache: "no-store",
  });
}

export function updateSandboxBinding(binding: SandboxBinding, options?: ApiRequestOptions) {
  return fetchJson<SandboxBinding>(
    `${BASE}/bindings/${binding.scope}/${encodeURIComponent(binding.target_id)}`,
    {
      ...options,
      init: {
        ...options?.init,
        method: "PUT",
        body: JSON.stringify({ binding, expected_version: binding.version }),
      },
    },
  );
}

export function previewSandboxSelection(
  context: SandboxSelectionContext,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxSelectionPreview>(`${BASE}/preview`, {
    ...options,
    init: { ...options?.init, method: "POST", body: JSON.stringify(context) },
  });
}

export function publishSandboxPolicy(
  policy: SandboxPolicy,
  expectedRevision: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<SandboxRevision>(`${BASE}/policies`, {
    ...options,
    init: {
      ...options?.init,
      method: "POST",
      body: JSON.stringify({ policy, expected_revision: expectedRevision }),
    },
  });
}
