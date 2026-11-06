import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

import {
  ACTIVE_WORKSPACE_COOKIE,
  LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE,
  promoteLegacyWorkspaceSelection,
  readActiveWorkspaceCookie,
  readCookie,
  readScopedCookie,
  resolveOfficeWorkspaceId,
} from "./route-bootstrap";
import { buildSettingsInitialStateForRoute } from "@/src/settings-routes.initial-state";
import { workspaceId } from "@/lib/types/ids";

const COOKIE = ACTIVE_WORKSPACE_COOKIE;
const OFFICE_COOKIE = LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE;

beforeEach(() => {
  for (const name of [COOKIE, OFFICE_COOKIE, `${COOKIE}_8443`, `${OFFICE_COOKIE}_8443`]) {
    document.cookie = `${name}=; path=/; max-age=0`;
  }
});

afterEach(() => vi.restoreAllMocks());

// @covers AC-AUTH-FIX-MULTI-INSTANCE-COOKIE-ISOLATION-001.9
describe("workspace cookie decoding recovery", () => {
  it.each(["%E0%A4%A", "%"])("treats malformed encoding %s as absent", (value) => {
    document.cookie = `${COOKIE}=${value}; path=/`;
    expect(readCookie(COOKIE)).toBeNull();
  });

  it("preserves UTF-8 decoding and empty values", () => {
    document.cookie = `${COOKIE}=${encodeURIComponent("工作 space/1")}; path=/`;
    expect(readCookie(COOKIE)).toBe("工作 space/1");
    document.cookie = `${COOKIE}=; path=/`;
    expect(readCookie(COOKIE)).toBe("");
    expect(readCookie("missing")).toBeNull();
  });

  it.each([
    ["first", "%", null],
    ["%", "last%20workspace", "last workspace"],
    ["first", "last", "last"],
  ])("uses only the last matching cookie (%s then %s)", (first, last, expected) => {
    vi.spyOn(document, "cookie", "get").mockReturnValue(
      `${COOKIE}=${first}; unrelated=%; ${COOKIE}=${last}`,
    );
    expect(readCookie(COOKIE)).toBe(expected);
  });

  it.each([COOKIE, OFFICE_COOKIE])(
    "promotes a valid legacy %s over a malformed scoped value once",
    (name) => {
      const id = "workspace/é";
      document.cookie = `${name}_8443=%E0%A4%A; path=/`;
      document.cookie = `${name}=${encodeURIComponent(id)}; path=/`;
      expect(readScopedCookie(name)).toBe(id);

      promoteLegacyWorkspaceSelection([{ id }], name);
      expect(readCookie(`${name}_8443`)).toBe(id);
      expect(readCookie(name)).toBe(id);

      document.cookie = `${name}=other; path=/`;
      promoteLegacyWorkspaceSelection([{ id }, { id: "other" }], name);
      expect(readScopedCookie(name)).toBe(id);
      expect(readCookie(name)).toBe("other");
    },
  );

  it("does not promote an unknown legacy workspace over a malformed scoped value", () => {
    document.cookie = `${COOKIE}_8443=%; path=/`;
    document.cookie = `${COOKIE}=unknown; path=/`;
    promoteLegacyWorkspaceSelection([{ id: "known" }]);
    expect(document.cookie).toContain(`${COOKIE}_8443=%`);
  });

  it("keeps a valid scoped selection ahead of a malformed legacy cookie", () => {
    document.cookie = `${COOKIE}_8443=known; path=/`;
    document.cookie = `${COOKIE}=%; path=/`;
    promoteLegacyWorkspaceSelection([{ id: "known" }]);
    expect(readActiveWorkspaceCookie()).toBe("known");
    expect(document.cookie).toContain(`${COOKIE}=%`);
  });

  it("falls through malformed general cookies to a valid Office cookie", () => {
    document.cookie = `${COOKIE}_8443=%; path=/`;
    document.cookie = `${COOKIE}=%E0%A4%A; path=/`;
    document.cookie = `${OFFICE_COOKIE}_8443=office-2; path=/`;
    expect(
      resolveOfficeWorkspaceId([{ id: "office-1" }, { id: "office-2" }], {
        generalWorkspaceId: readActiveWorkspaceCookie(),
        officeWorkspaceId: readScopedCookie(OFFICE_COOKIE),
        settingsWorkspaceId: "office-1",
      }),
    ).toBe("office-2");
  });

  it.each(["office-2", "unknown"])(
    "falls through malformed Office cookies to preference %s or the first workspace",
    (preference) => {
      document.cookie = `${OFFICE_COOKIE}_8443=%; path=/`;
      document.cookie = `${OFFICE_COOKIE}=%E0%A4%A; path=/`;
      promoteLegacyWorkspaceSelection([{ id: "office-1" }, { id: "office-2" }], OFFICE_COOKIE);
      expect(
        resolveOfficeWorkspaceId([{ id: "office-1" }, { id: "office-2" }], {
          officeWorkspaceId: readScopedCookie(OFFICE_COOKIE),
          settingsWorkspaceId: preference,
        }),
      ).toBe(preference === "office-2" ? "office-2" : "office-1");
    },
  );
});

describe("settings cookie recovery before hydration", () => {
  it.each(["ws-2", "unknown"])(
    "builds settings hydration with malformed cookies and preference %s",
    (preference) => {
      document.cookie = `${COOKIE}_8443=%; path=/`;
      document.cookie = `${COOKIE}=%E0%A4%A; path=/`;
      document.cookie = `${OFFICE_COOKIE}=office-selection; path=/`;
      const state = buildSettingsInitialStateForRoute({
        workspaces: ["ws-1", "ws-2"].map((id) => ({
          id: workspaceId(id),
          name: id,
          owner_id: "owner-1",
          acp_idle_suspension_enabled: false,
          acp_idle_timeout_minutes: 120,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        })),
        executors: [],
        agents: [],
        discoveryAgents: [],
        availableAgents: [],
        availableTools: [],
        userSettingsResponse: {
          settings: {
            user_id: "owner-1",
            workspace_id: workspaceId(preference),
            repository_ids: [],
            updated_at: "2026-01-01T00:00:00Z",
          },
        },
      });
      expect(state.workspaces?.activeId).toBe(preference === "ws-2" ? "ws-2" : "ws-1");
      expect(state.userSettings?.workspaceId).toBe(state.workspaces?.activeId);
      expect(document.cookie).toContain(`${COOKIE}_8443=%`);
      expect(document.cookie).toContain(`${COOKIE}=%E0%A4%A`);
      expect(readCookie(OFFICE_COOKIE)).toBe("office-selection");
    },
  );
});
