import { expect, type BrowserContext } from "@playwright/test";

/**
 * Helpers for the opt-in authentication E2E suite.
 *
 * All calls go through a BrowserContext's request API so the kandev_session
 * cookie set by login/setup/invite-accept is shared with pages opened from the
 * same context (matching how the real SPA authenticates).
 */

export type Credentials = {
  email: string;
  password: string;
  displayName?: string;
};

export async function setupAdmin(
  context: BrowserContext,
  baseUrl: string,
  creds: Credentials,
): Promise<void> {
  const res = await context.request.post(`${baseUrl}/api/v1/auth/setup`, {
    data: {
      email: creds.email,
      password: creds.password,
      display_name: creds.displayName ?? "",
    },
  });
  expect(res.ok(), `setup failed: ${res.status()} ${await res.text()}`).toBeTruthy();
}

export async function login(
  context: BrowserContext,
  baseUrl: string,
  creds: Credentials,
): Promise<void> {
  const res = await context.request.post(`${baseUrl}/api/v1/auth/login`, {
    data: { email: creds.email, password: creds.password },
  });
  expect(res.ok(), `login failed: ${res.status()} ${await res.text()}`).toBeTruthy();
}

/** Mints an invite as the (admin) context and returns the raw token. */
export async function createInviteToken(
  adminContext: BrowserContext,
  baseUrl: string,
  options: { email?: string; role?: "member" | "admin" } = {},
): Promise<string> {
  const res = await adminContext.request.post(`${baseUrl}/api/v1/auth/invites`, {
    data: { email: options.email ?? "", role: options.role ?? "member" },
  });
  expect(res.status(), await res.text()).toBe(201);
  const body = (await res.json()) as { url: string };
  const token = new URL(body.url, baseUrl).searchParams.get("token");
  expect(token, "invite URL must carry a token").toBeTruthy();
  return token as string;
}

export async function acceptInvite(
  context: BrowserContext,
  baseUrl: string,
  token: string,
  creds: Credentials,
): Promise<void> {
  const res = await context.request.post(`${baseUrl}/api/v1/auth/invites/accept`, {
    data: {
      token,
      email: creds.email,
      password: creds.password,
      display_name: creds.displayName ?? "",
    },
  });
  expect(res.ok(), `invite accept failed: ${res.status()} ${await res.text()}`).toBeTruthy();
}

/** Fetches the boot payload the SPA would hydrate from. */
export async function fetchBootState(
  context: BrowserContext,
  baseUrl: string,
): Promise<{ initialState: Record<string, unknown> }> {
  const res = await context.request.get(`${baseUrl}/api/v1/app-state?path=/`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()) as { initialState: Record<string, unknown> };
}

export async function mintPAT(
  context: BrowserContext,
  baseUrl: string,
  name: string,
): Promise<string> {
  const res = await context.request.post(`${baseUrl}/api/v1/auth/tokens`, { data: { name } });
  expect(res.status(), await res.text()).toBe(201);
  const body = (await res.json()) as { token: string };
  expect(body.token).toMatch(/^kandev_pat_/);
  return body.token;
}
