import { describe, expect, it } from "vitest";

import type { AzureDevOpsConfig } from "@/lib/types/azure-devops";

import {
  canSubmitForm,
  configToForm,
  EMPTY_FORM,
  requestFromForm,
  savedPATMatches,
  startRequestFromForm,
  signedInWithMicrosoft,
} from "./azure-devops-settings-form";

const ACME_URL = "https://dev.azure.com/acme";
const PAGE_ORIGIN = "http://localhost:48429";

const BASE_CONFIG: AzureDevOpsConfig = {
  workspaceId: "ws-1",
  organizationUrl: ACME_URL,
  defaultProjectId: "p-1",
  defaultProjectName: "Platform",
  authMethod: "pat",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

const OAUTH_CONFIG: AzureDevOpsConfig = {
  ...BASE_CONFIG,
  authMethod: "oauth",
  oauthAccount: "Ana Silva",
};

describe("Azure DevOps settings form", () => {
  it("defaults a new connection to the PAT method", () => {
    expect(EMPTY_FORM.authMethod).toBe("pat");
    expect(configToForm(null).authMethod).toBe("pat");
  });

  it("restores the stored method", () => {
    expect(configToForm(OAUTH_CONFIG)).toMatchObject({
      authMethod: "oauth",
      organizationUrl: ACME_URL,
      defaultProjectName: "Platform",
    });
  });

  it("sends the PAT only for the PAT method", () => {
    const form = { ...configToForm(BASE_CONFIG), pat: "secret" };
    expect(requestFromForm(form, BASE_CONFIG)).toMatchObject({ authMethod: "pat", pat: "secret" });
    expect(requestFromForm({ ...form, authMethod: "oauth" }, BASE_CONFIG)).toEqual({
      organizationUrl: ACME_URL,
      defaultProjectId: "p-1",
      defaultProjectName: "Platform",
      authMethod: "oauth",
    });
  });

  it("reuses a saved PAT only for a PAT connection of the same organization", () => {
    expect(savedPATMatches(BASE_CONFIG, configToForm(BASE_CONFIG))).toBe(true);
    expect(
      savedPATMatches(OAUTH_CONFIG, { ...configToForm(OAUTH_CONFIG), authMethod: "pat" }),
    ).toBe(false);
    expect(
      savedPATMatches(BASE_CONFIG, {
        ...configToForm(BASE_CONFIG),
        organizationUrl: "https://dev.azure.com/other",
      }),
    ).toBe(false);
  });

  it("treats a Microsoft connection as signed in only for its organization", () => {
    const form = configToForm(OAUTH_CONFIG);
    expect(signedInWithMicrosoft(OAUTH_CONFIG, form)).toBe(true);
    expect(
      signedInWithMicrosoft(OAUTH_CONFIG, { ...form, organizationUrl: "https://dev.azure.com/x" }),
    ).toBe(false);
    expect(signedInWithMicrosoft({ ...OAUTH_CONFIG, hasSecret: false }, form)).toBe(false);
    expect(signedInWithMicrosoft(BASE_CONFIG, { ...form, authMethod: "oauth" })).toBe(false);
  });

  it("allows submitting PAT forms with a credential and Microsoft forms after sign-in", () => {
    expect(canSubmitForm(null, { ...EMPTY_FORM, organizationUrl: ACME_URL })).toBe(false);
    expect(
      canSubmitForm(null, {
        ...EMPTY_FORM,
        organizationUrl: ACME_URL,
        pat: "secret",
      }),
    ).toBe(true);
    expect(canSubmitForm(BASE_CONFIG, configToForm(BASE_CONFIG))).toBe(true);
    expect(canSubmitForm(BASE_CONFIG, { ...configToForm(BASE_CONFIG), authMethod: "oauth" })).toBe(
      false,
    );
    expect(canSubmitForm(OAUTH_CONFIG, configToForm(OAUTH_CONFIG))).toBe(true);
  });
});

describe("Microsoft application fields", () => {
  it("restores the workspace client ID of a Microsoft connection", () => {
    expect(
      configToForm({ ...OAUTH_CONFIG, oauthClientId: "04b07795-8ddb-461a-bbee-02f9e1bf7b46" }),
    ).toMatchObject({ oauthClientId: "04b07795-8ddb-461a-bbee-02f9e1bf7b46" });
    expect(configToForm(BASE_CONFIG)).toMatchObject({ oauthClientId: "" });
  });

  it("redirects a workspace application back to the current page origin", () => {
    const form = { ...configToForm(OAUTH_CONFIG), oauthClientId: " client-x " };
    expect(startRequestFromForm(form, PAGE_ORIGIN)).toEqual({
      organizationUrl: ACME_URL,
      defaultProjectId: "p-1",
      defaultProjectName: "Platform",
      clientId: "client-x",
      redirectUrl: PAGE_ORIGIN,
    });
  });

  it("redirects the installation default application back to the page origin too", () => {
    const form = { ...configToForm(OAUTH_CONFIG), oauthClientId: "" };
    expect(startRequestFromForm(form, PAGE_ORIGIN)).toEqual({
      organizationUrl: ACME_URL,
      defaultProjectId: "p-1",
      defaultProjectName: "Platform",
      redirectUrl: PAGE_ORIGIN,
    });
  });
});
