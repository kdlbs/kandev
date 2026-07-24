import { fetchJson, type ApiRequestOptions } from "../client";
import type { AuthMode, AuthUser } from "@/lib/state/slices/auth/types";

export type { AuthMode, AuthUser };

// Mirrors apps/backend/internal/auth/httpapi (see the package doc comment in
// handlers.go for the full route table) and apps/backend/internal/auth/state.go
// (StatePayload). Kept in one file since the whole auth HTTP surface is one
// small package on the backend too.

const AUTH_BASE = "/api/v1/auth";

export type AuthState = {
  mode: AuthMode;
  authenticated: boolean;
  user?: AuthUser;
};

export type AuthSession = {
  id: string;
  created_at: string;
  last_seen_at: string;
  user_agent: string;
  ip: string;
  current: boolean;
};

export type ApiToken = {
  id: string;
  user_id: string;
  name: string;
  created_at: string;
  last_used_at?: string | null;
  expires_at?: string | null;
};

export type Invite = {
  id: string;
  email?: string;
  role: string;
  created_by: string;
  expires_at: string;
  accepted_at?: string | null;
};

export type InvitePreview = {
  email?: string;
  role: string;
  expires_at: string;
};

function post<T>(path: string, body?: unknown, options?: ApiRequestOptions): Promise<T> {
  return fetchJson<T>(`${AUTH_BASE}${path}`, {
    ...options,
    init: {
      method: "POST",
      ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
      ...(options?.init ?? {}),
    },
  });
}

function patch<T>(path: string, body: unknown, options?: ApiRequestOptions): Promise<T> {
  return fetchJson<T>(`${AUTH_BASE}${path}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(body), ...(options?.init ?? {}) },
  });
}

function del<T>(path: string, options?: ApiRequestOptions): Promise<T> {
  return fetchJson<T>(`${AUTH_BASE}${path}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

// --- session lifecycle ---

export function fetchAuthState(options?: ApiRequestOptions): Promise<AuthState> {
  return fetchJson<AuthState>(`${AUTH_BASE}/me`, options);
}

export function setup(
  body: { email: string; password: string; display_name: string },
  options?: ApiRequestOptions,
): Promise<{ user: AuthUser }> {
  return post("/setup", body, options);
}

export function login(
  body: { email: string; password: string },
  options?: ApiRequestOptions,
): Promise<{ user: AuthUser }> {
  return post("/login", body, options);
}

export function logout(options?: ApiRequestOptions): Promise<void> {
  return post("/logout", undefined, options);
}

export function changePassword(
  body: { current_password: string; new_password: string },
  options?: ApiRequestOptions,
): Promise<void> {
  return patch("/password", body, options);
}

// --- sessions ---

export function listSessions(options?: ApiRequestOptions): Promise<{ sessions: AuthSession[] }> {
  return fetchJson<{ sessions: AuthSession[] }>(`${AUTH_BASE}/sessions`, options);
}

export function revokeSession(id: string, options?: ApiRequestOptions): Promise<void> {
  return del(`/sessions/${id}`, options);
}

// --- personal access tokens ---

export function listTokens(options?: ApiRequestOptions): Promise<{ tokens: ApiToken[] }> {
  return fetchJson<{ tokens: ApiToken[] }>(`${AUTH_BASE}/tokens`, options);
}

export function mintToken(
  body: { name: string; ttl_hours?: number },
  options?: ApiRequestOptions,
): Promise<{ token: string; record: ApiToken }> {
  return post("/tokens", body, options);
}

export function revokeToken(id: string, options?: ApiRequestOptions): Promise<void> {
  return del(`/tokens/${id}`, options);
}

// --- invites ---

export function listInvites(options?: ApiRequestOptions): Promise<{ invites: Invite[] }> {
  return fetchJson<{ invites: Invite[] }>(`${AUTH_BASE}/invites`, options);
}

export function createInvite(
  body: { email?: string; role: string; ttl_hours?: number },
  options?: ApiRequestOptions,
): Promise<{ invite: Invite; url: string }> {
  return post("/invites", body, options);
}

export function deleteInvite(id: string, options?: ApiRequestOptions): Promise<void> {
  return del(`/invites/${id}`, options);
}

export function previewInvite(token: string, options?: ApiRequestOptions): Promise<InvitePreview> {
  return fetchJson<InvitePreview>(
    `${AUTH_BASE}/invites/preview?token=${encodeURIComponent(token)}`,
    options,
  );
}

export function acceptInvite(
  body: { token: string; email: string; password: string; display_name: string },
  options?: ApiRequestOptions,
): Promise<{ user: AuthUser }> {
  return post("/invites/accept", body, options);
}

// --- admin user management ---

export function listUsers(options?: ApiRequestOptions): Promise<{ users: AuthUser[] }> {
  return fetchJson<{ users: AuthUser[] }>("/api/v1/users", options);
}

export function createUser(
  body: { email: string; password: string; display_name: string; role: string },
  options?: ApiRequestOptions,
): Promise<{ user: AuthUser }> {
  return fetchJson<{ user: AuthUser }>("/api/v1/users", {
    ...options,
    init: { method: "POST", body: JSON.stringify(body), ...(options?.init ?? {}) },
  });
}

export function updateUser(
  id: string,
  body: { role?: string; status?: string },
  options?: ApiRequestOptions,
): Promise<{ user: AuthUser }> {
  return fetchJson<{ user: AuthUser }>(`/api/v1/users/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(body), ...(options?.init ?? {}) },
  });
}
