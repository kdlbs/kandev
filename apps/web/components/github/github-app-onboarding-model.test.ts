import { describe, expect, it, vi } from "vitest";
import {
  callbackNotice,
  defaultVisibility,
  githubAppSettingsURL,
  normalizePublicBaseUrl,
  submitManifestToGitHub,
  validateImportSecrets,
  validateRegistrationBasics,
} from "./github-app-onboarding-model";

describe("GitHub App onboarding model", () => {
  it("requires a public HTTPS origin", () => {
    for (const value of ["http://kandev.example", "https://localhost", "https://host.test/path"]) {
      const errors = {};
      expect(normalizePublicBaseUrl(value, errors)).toBeNull();
      expect(errors).toHaveProperty("publicBaseUrl");
    }
    expect(normalizePublicBaseUrl("https://kandev.example/", {})).toBe("https://kandev.example");
  });

  it("validates registration identity and imported secrets", () => {
    const basics = validateRegistrationBasics({
      displayName: " Work App ",
      ownerType: "Organization",
      ownerLogin: "acme-inc",
      publicBaseUrl: "https://kandev.example",
    });
    expect(basics).toMatchObject({ displayName: "Work App", ownerLogin: "acme-inc", errors: {} });
    expect(
      validateImportSecrets({
        appId: "42",
        clientId: "client",
        clientSecret: "secret",
        privateKey: "key",
        webhookSecret: "hook",
        slug: "work-app",
      }).errors,
    ).toEqual({});
    expect(defaultVisibility).toBe("private");
  });

  it("maps callback results and account-specific App settings URLs", () => {
    expect(callbackNotice("app_connected").tone).toBe("success");
    expect(callbackNotice("github_app_registration_cancelled").tone).toBe("info");
    expect(callbackNotice("unknown").tone).toBe("error");
    expect(githubAppSettingsURL("Organization", "acme", "work-app")).toBe(
      "https://github.com/organizations/acme/settings/apps/work-app",
    );
    expect(githubAppSettingsURL("User", "octocat", "personal-app")).toBe(
      "https://github.com/settings/apps/personal-app",
    );
  });

  it("posts the generated manifest to GitHub without exposing it in a URL", () => {
    const submit = vi
      .spyOn(HTMLFormElement.prototype, "submit")
      .mockImplementation(() => undefined);
    submitManifestToGitHub("https://github.com/settings/apps/new", { name: "Kandev" });
    const form = document.body.querySelector("form");
    expect(form?.method).toBe("POST");
    expect(form?.action).toBe("https://github.com/settings/apps/new");
    expect(form?.querySelector("input")?.value).toBe('{"name":"Kandev"}');
    submit.mockRestore();
    form?.remove();
  });
});
