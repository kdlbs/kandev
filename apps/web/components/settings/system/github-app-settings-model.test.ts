import { describe, expect, it } from "vitest";

import {
  callbackNoticeForResult,
  manifestFormFields,
  validateDeploymentAppSetup,
} from "./github-app-settings-model";

describe("GitHub App setup validation", () => {
  it("accepts a GitHub owner and canonical public HTTPS origin", () => {
    expect(
      validateDeploymentAppSetup({
        ownerType: "organization",
        ownerLogin: "acme-platform",
        publicBaseUrl: "https://Kandev.Example/",
      }),
    ).toEqual({
      values: {
        owner_type: "organization",
        owner_login: "acme-platform",
        public_base_url: "https://kandev.example",
      },
      errors: {},
    });
  });

  it.each([
    ["http://kandev.example", "Enter a public HTTPS origin."],
    ["https://localhost:8080", "Enter a public host, not localhost."],
    ["https://kandev.example/path", "Enter an origin without a path, query, or fragment."],
  ])("rejects an invalid public URL %s", (publicBaseUrl, message) => {
    const result = validateDeploymentAppSetup({
      ownerType: "user",
      ownerLogin: "octocat",
      publicBaseUrl,
    });

    expect(result.errors.publicBaseUrl).toBe(message);
  });

  it("requires a valid owner login", () => {
    const result = validateDeploymentAppSetup({
      ownerType: "organization",
      ownerLogin: "-not valid-",
      publicBaseUrl: "https://kandev.example",
    });

    expect(result.errors.ownerLogin).toBe("Enter the GitHub organization login.");
  });
});

describe("GitHub App manifest handoff", () => {
  it("posts only the server-generated manifest to GitHub", () => {
    expect(manifestFormFields({ name: "Kandev acme", public: true })).toEqual({
      manifest: '{"name":"Kandev acme","public":true}',
    });
  });

  it("maps callback result codes without exposing callback details", () => {
    expect(callbackNoticeForResult("connected")).toMatchObject({
      tone: "success",
      title: "GitHub App created",
    });
    expect(callbackNoticeForResult("github_app_invalid_callback")).toMatchObject({
      tone: "error",
      title: "GitHub App setup was not completed",
    });
    expect(callbackNoticeForResult("github_app_registration_cancelled")).toMatchObject({
      tone: "info",
      title: "GitHub App setup cancelled",
    });
    expect(callbackNoticeForResult("github_app_environment_read_only")).toMatchObject({
      tone: "error",
      title: "GitHub App setup is managed externally",
    });
    expect(callbackNoticeForResult("unknown")).toMatchObject({
      tone: "error",
      title: "GitHub App setup failed",
    });
    expect(callbackNoticeForResult("")).toBeNull();
  });
});
