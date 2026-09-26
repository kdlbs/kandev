import { beforeAll, describe, expect, it } from "vitest";

import { activateLocale, i18n } from "@/lib/i18n";
import type { JiraConfig } from "@/lib/types/jira";
import { configToForm, deriveFormState, type FormState } from "./jira-form-state";

const t = i18n.t;

beforeAll(async () => {
  await activateLocale("en");
});

function config(overrides: Partial<JiraConfig> = {}): JiraConfig {
  return {
    siteUrl: "https://acme.atlassian.net",
    email: "user@example.com",
    authMethod: "oauth",
    instanceType: "cloud",
    defaultProjectKey: "ENG",
    hasSecret: true,
    clientId: "client-1",
    lastOk: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function oauthForm(overrides: Partial<FormState> = {}): FormState {
  return {
    ...configToForm(config()),
    ...overrides,
  };
}

function derive(saved: JiraConfig | null, form: FormState) {
  return deriveFormState({ config: saved, form, saving: false, loading: false }, t);
}

describe("deriveFormState OAuth credential identity", () => {
  it("requires a connection before a fresh OAuth form can be saved or tested", () => {
    const state = derive(null, oauthForm());
    expect(state.disableSave).toBe(true);
    expect(state.disableTest).toBe(true);
  });

  it("does not reuse an API-token secret after switching to OAuth", () => {
    const saved = config({ authMethod: "api_token", clientId: undefined });
    const state = derive(saved, oauthForm());
    expect(state.disableSave).toBe(true);
    expect(state.disableTest).toBe(true);
  });

  it("requires reconnecting after the OAuth site changes", () => {
    const saved = config();
    const state = derive(saved, oauthForm({ siteUrl: "https://other.atlassian.net" }));
    expect(state.savedSecretMatchesMode).toBe(false);
    expect(state.disableSave).toBe(true);
    expect(state.disableTest).toBe(true);
  });

  it("allows a saved OAuth connection whose full identity still matches", () => {
    const saved = config();
    const state = derive(saved, oauthForm());
    expect(state.savedSecretMatchesMode).toBe(true);
    expect(state.disableSave).toBe(false);
    expect(state.disableTest).toBe(false);
  });
});
