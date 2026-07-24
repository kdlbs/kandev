import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  fetchAuthState,
  setup,
  login,
  logout,
  changePassword,
  listSessions,
  revokeSession,
  listTokens,
  mintToken,
  revokeToken,
  listInvites,
  createInvite,
  previewInvite,
  acceptInvite,
  listUsers,
  createUser,
  updateUser,
} from "./auth-api";

const BASE = "http://api.test/api/v1/auth";
const TEST_PASSWORD = "password123";
const INVITEE_EMAIL = "invitee@x.com";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

function noContentResponse(): Response {
  return new Response(null, { status: 204 });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

function method(): string {
  return (lastCall().init?.method ?? "GET").toUpperCase();
}

function bodyJson(): unknown {
  const raw = lastCall().init?.body;
  return raw ? JSON.parse(String(raw)) : undefined;
}

describe("fetchAuthState", () => {
  it("GETs /me and returns the StatePayload shape", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ mode: "enabled", authenticated: false }));
    const state = await fetchAuthState();
    expect(lastCall().url).toBe(`${BASE}/me`);
    expect(method()).toBe("GET");
    expect(state.mode).toBe("enabled");
    expect(state.authenticated).toBe(false);
  });
});

describe("setup", () => {
  it("POSTs credentials to /setup", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        user: { id: "u1", email: "a@b.com", display_name: "A", role: "admin", status: "active" },
      }),
    );
    const res = await setup({ email: "a@b.com", password: TEST_PASSWORD, display_name: "A" });
    expect(lastCall().url).toBe(`${BASE}/setup`);
    expect(method()).toBe("POST");
    expect(bodyJson()).toEqual({ email: "a@b.com", password: TEST_PASSWORD, display_name: "A" });
    expect(res.user.role).toBe("admin");
  });
});

describe("login", () => {
  it("POSTs credentials to /login", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        user: { id: "u1", email: "a@b.com", display_name: "A", role: "member", status: "active" },
      }),
    );
    await login({ email: "a@b.com", password: TEST_PASSWORD });
    expect(lastCall().url).toBe(`${BASE}/login`);
    expect(method()).toBe("POST");
  });
});

describe("logout", () => {
  it("POSTs to /logout and resolves on 204", async () => {
    fetchSpy.mockResolvedValueOnce(noContentResponse());
    await expect(logout()).resolves.toBeUndefined();
    expect(lastCall().url).toBe(`${BASE}/logout`);
    expect(method()).toBe("POST");
  });
});

describe("changePassword", () => {
  it("PATCHes /password", async () => {
    fetchSpy.mockResolvedValueOnce(noContentResponse());
    await changePassword({ current_password: "old12345", new_password: "new123456" });
    expect(lastCall().url).toBe(`${BASE}/password`);
    expect(method()).toBe("PATCH");
  });
});

describe("sessions", () => {
  it("lists and revokes sessions", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        sessions: [
          {
            id: "s1",
            created_at: "now",
            last_seen_at: "now",
            user_agent: "ua",
            ip: "1.2.3.4",
            current: true,
          },
        ],
      }),
    );
    const res = await listSessions();
    expect(lastCall().url).toBe(`${BASE}/sessions`);
    expect(res.sessions).toHaveLength(1);

    fetchSpy.mockResolvedValueOnce(noContentResponse());
    await revokeSession("s1");
    expect(lastCall().url).toBe(`${BASE}/sessions/s1`);
    expect(method()).toBe("DELETE");
  });
});

describe("tokens", () => {
  it("mints, lists, and revokes tokens", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        token: "raw-token",
        record: { id: "t1", user_id: "u1", name: "ci", created_at: "now" },
      }),
    );
    const minted = await mintToken({ name: "ci", ttl_hours: 24 });
    expect(lastCall().url).toBe(`${BASE}/tokens`);
    expect(method()).toBe("POST");
    expect(minted.token).toBe("raw-token");

    fetchSpy.mockResolvedValueOnce(jsonResponse({ tokens: [minted.record] }));
    const listed = await listTokens();
    expect(listed.tokens).toHaveLength(1);

    fetchSpy.mockResolvedValueOnce(noContentResponse());
    await revokeToken("t1");
    expect(lastCall().url).toBe(`${BASE}/tokens/t1`);
    expect(method()).toBe("DELETE");
  });
});

describe("invites", () => {
  it("creates, lists, previews, and accepts invites", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        invite: { id: "i1", role: "member", created_by: "u1", expires_at: "later" },
        url: "/invite?token=raw",
      }),
    );
    const created = await createInvite({ role: "member", ttl_hours: 48 });
    expect(lastCall().url).toBe(`${BASE}/invites`);
    expect(created.url).toBe("/invite?token=raw");

    fetchSpy.mockResolvedValueOnce(jsonResponse({ invites: [created.invite] }));
    const listed = await listInvites();
    expect(listed.invites).toHaveLength(1);

    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ email: INVITEE_EMAIL, role: "member", expires_at: "later" }),
    );
    const preview = await previewInvite("raw");
    expect(lastCall().url).toBe(`${BASE}/invites/preview?token=raw`);
    expect(preview.role).toBe("member");

    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        user: {
          id: "u2",
          email: INVITEE_EMAIL,
          display_name: "Invitee",
          role: "member",
          status: "active",
        },
      }),
    );
    const accepted = await acceptInvite({
      token: "raw",
      email: INVITEE_EMAIL,
      password: TEST_PASSWORD,
      display_name: "Invitee",
    });
    expect(lastCall().url).toBe(`${BASE}/invites/accept`);
    expect(accepted.user.email).toBe(INVITEE_EMAIL);
  });
});

describe("admin users", () => {
  it("lists, creates, and updates users", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        users: [{ id: "u1", email: "a@b.com", display_name: "A", role: "admin", status: "active" }],
      }),
    );
    const users = await listUsers();
    expect(lastCall().url).toBe("http://api.test/api/v1/users");
    expect(users.users).toHaveLength(1);

    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        user: { id: "u2", email: "c@d.com", display_name: "C", role: "member", status: "active" },
      }),
    );
    const created = await createUser({
      email: "c@d.com",
      password: TEST_PASSWORD,
      display_name: "C",
      role: "member",
    });
    expect(method()).toBe("POST");
    expect(created.user.email).toBe("c@d.com");

    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        user: { id: "u2", email: "c@d.com", display_name: "C", role: "admin", status: "active" },
      }),
    );
    const updated = await updateUser("u2", { role: "admin" });
    expect(lastCall().url).toBe("http://api.test/api/v1/users/u2");
    expect(method()).toBe("PATCH");
    expect(updated.user.role).toBe("admin");
  });
});
