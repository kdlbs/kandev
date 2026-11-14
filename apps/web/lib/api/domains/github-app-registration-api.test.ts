import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
import {
  deleteGitHubAppRegistration,
  fetchGitHubAppRegistrations,
  importGitHubAppRegistration,
  prepareGitHubAppImport,
  renameGitHubAppRegistration,
  startGitHubAppManifest,
} from "./github-api";
import * as githubAuthApi from "./github-auth-api";
const workspaceID = "ws/1";
const appRegistrationID = "registration/one";
const publicKandevURL = "https://kandev.example";
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

describe("workspace GitHub App registrations", () => {
  it("exposes the registration catalog API", () => {
    expect(githubAuthApi).toMatchObject({
      fetchGitHubAppRegistrations: expect.any(Function),
      startGitHubAppManifest: expect.any(Function),
      prepareGitHubAppImport: expect.any(Function),
      importGitHubAppRegistration: expect.any(Function),
      renameGitHubAppRegistration: expect.any(Function),
      deleteGitHubAppRegistration: expect.any(Function),
    });
  });

  it("uses workspace-scoped catalog and registration lifecycle routes", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ workspace_id: workspaceID, registrations: [] }))
      .mockResolvedValueOnce(
        jsonResponse({
          registration_id: appRegistrationID,
          workspace_id: workspaceID,
          state: "state",
          expires_at: "2026-07-20T22:00:00Z",
          revision: 1,
          registration_url: "https://github.com/organizations/acme/settings/apps/new",
          manifest: { name: "Kandev acme" },
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          registration_id: appRegistrationID,
          public_base_url: publicKandevURL,
          webhook_url: "https://kandev.example/webhook",
        }),
      )
      .mockResolvedValueOnce(jsonResponse({ id: appRegistrationID }))
      .mockResolvedValueOnce(jsonResponse({ id: appRegistrationID, display_name: "Renamed" }))
      .mockResolvedValueOnce(jsonResponse({ deleted: true }));

    await fetchGitHubAppRegistrations(workspaceID);
    await startGitHubAppManifest({
      workspace_id: workspaceID,
      display_name: "Work App",
      owner_type: "organization",
      owner_login: "acme",
      visibility: "private",
      public_base_url: publicKandevURL,
    });
    await prepareGitHubAppImport({
      workspace_id: workspaceID,
      public_base_url: publicKandevURL,
    });
    await importGitHubAppRegistration({
      registration_id: "registration-1",
      workspace_id: workspaceID,
      display_name: "Existing App",
      github_host: "github.com",
      app_id: 42,
      client_id: "Iv1.client",
      client_secret: "client-secret",
      private_key: "private-key",
      webhook_secret: "webhook-secret",
      slug: "existing-app",
      owner_login: "acme",
      owner_type: "Organization",
      visibility: "private",
      public_base_url: publicKandevURL,
    });
    await renameGitHubAppRegistration(appRegistrationID, "Renamed");
    await deleteGitHubAppRegistration(appRegistrationID);

    expect(fetchSpy.mock.calls.map((call) => [String(call[0]), call[1]?.method])).toEqual([
      ["http://api.test/api/v1/github/app/registrations?workspace_id=ws%2F1", undefined],
      ["http://api.test/api/v1/github/app/registrations/manifest/start", "POST"],
      ["http://api.test/api/v1/github/app/registrations/import/prepare", "POST"],
      ["http://api.test/api/v1/github/app/registrations/import", "POST"],
      ["http://api.test/api/v1/github/app/registrations/registration%2Fone", "PATCH"],
      ["http://api.test/api/v1/github/app/registrations/registration%2Fone", "DELETE"],
    ]);
    expect(fetchSpy.mock.calls[0]?.[1]?.cache).toBe("no-store");
    expect(JSON.parse(String(fetchSpy.mock.calls[1]?.[1]?.body))).toEqual({
      workspace_id: workspaceID,
      display_name: "Work App",
      owner_type: "organization",
      owner_login: "acme",
      visibility: "private",
      public_base_url: publicKandevURL,
    });
    expect(JSON.parse(String(fetchSpy.mock.calls[2]?.[1]?.body))).toEqual({
      workspace_id: workspaceID,
      public_base_url: publicKandevURL,
    });
    expect(JSON.parse(String(fetchSpy.mock.calls[4]?.[1]?.body))).toEqual({
      display_name: "Renamed",
    });
  });
});
