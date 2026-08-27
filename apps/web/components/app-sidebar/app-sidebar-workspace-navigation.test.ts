import { beforeEach, describe, expect, it, vi } from "vitest";
import { LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE } from "@/lib/routing/route-bootstrap";

// The write path scopes names with the API-origin port; pin it so the
// assertions are deterministic.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

import {
  rememberWorkspaceSelection,
  rememberWorkspaceSelectionById,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
const kanban = { id: "kanban-1", office_workflow_id: "" };
const office = { id: "office-1", office_workflow_id: "wf-office" };
const officeWithReservedChars = {
  id: "office/2;mode",
  office_workflow_id: "wf-office-reserved",
};

// Reads a cookie's value, treating an empty entry (lingering `name=` after an
// expired write) as absent — the same semantics as the production reader
// (readCookie: "an empty cookie value is treated as absent").
function cookieValue(name: string): string | null {
  const prefix = `${name}=`;
  const entry = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(prefix));
  if (!entry) return null;
  const value = decodeURIComponent(entry.slice(prefix.length));
  return value || null;
}

describe("app sidebar workspace navigation", () => {
  beforeEach(() => {
    document.cookie = "kandev-active-workspace=; path=/; max-age=0";
    document.cookie = "office-active-workspace=; path=/; max-age=0";
    document.cookie = "kandev-active-workspace_8443=; path=/; max-age=0";
    document.cookie = "office-active-workspace_8443=; path=/; max-age=0";
  });

  it("routes workspace home by active workspace type", () => {
    expect(workspaceHomeHref(kanban)).toBe("/?home=overview&workspaceId=kanban-1");
    expect(workspaceHomeHref(office)).toBe("/office?workspaceId=office-1");
    expect(workspaceHomeHref(undefined)).toBe("/?home=overview");
  });

  it("records the active workspace in one write under the port-scoped names", () => {
    rememberWorkspaceSelection(kanban);
    rememberWorkspaceSelection(office);

    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    // The legacy unprefixed names are read-only fallback — never written.
    // Assert on VALUES, not presence: some environments keep an empty
    // `name=` entry after a max-age=0 cleanup, and an empty value is
    // equivalent to absent (readCookie treats it as such).
    expect(cookieValue(ACTIVE_WORKSPACE_COOKIE)).toBeNull();
    expect(cookieValue(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)).toBeNull();
  });

  it("does not write the legacy office cookie for a kanban selection", () => {
    rememberWorkspaceSelection(office);
    rememberWorkspaceSelection(kanban);

    // The office boot paths read the legacy cookie to pick an office workspace
    // when the unified cookie names a kanban board, so a kanban selection must
    // leave it pointing at the office workspace last used.
    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=kanban-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
  });

  it("writes scoped active and office workspace cookies with an encoded id", () => {
    rememberWorkspaceSelection(officeWithReservedChars);

    expect(document.cookie).toContain(
      `${ACTIVE_WORKSPACE_COOKIE}_8443=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
    expect(document.cookie).toContain(
      `${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
  });

  it("records a workspace known only by id and kind", () => {
    // The setup wizard path: the create response returns an id and nothing
    // else, so there is no record to pass.
    rememberWorkspaceSelectionById("office-new", "office");

    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=office-new`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-new`);
    expect(cookieValue(ACTIVE_WORKSPACE_COOKIE)).toBeNull();
    expect(cookieValue(LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE)).toBeNull();
  });

  it("leaves a pre-existing legacy cookie untouched when the scoped twin is written", () => {
    // Pre-upgrade jar (or a default-port instance sharing the host): the
    // unprefixed names hold a value that must survive — it is either the
    // migration fallback for other instances or another instance's live
    // selection. The write path must not scrub it (spec: no proactive legacy
    // cookie deletion).
    document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=pre-upgrade; path=/`;
    document.cookie = `${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=pre-upgrade; path=/`;

    rememberWorkspaceSelection(office);

    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}_8443=office-1`);
    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}=pre-upgrade`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=pre-upgrade`);
  });
});
