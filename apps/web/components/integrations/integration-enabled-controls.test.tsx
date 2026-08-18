import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import type { ComponentType } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { AzureDevOpsEnabledControl } from "@/components/azure-devops/azure-devops-enabled-control";
import { GitHubEnabledControl } from "@/components/github/github-enabled-control";
import { GitLabEnabledControl } from "@/components/gitlab/gitlab-enabled-control";
import { JiraEnabledControl } from "@/components/jira/jira-enabled-control";
import { LinearEnabledControl } from "@/components/linear/linear-enabled-control";
import { SentryEnabledControl } from "@/components/sentry/sentry-enabled-control";
import { INTEGRATION_ENABLED_KEYS } from "@/lib/integrations/integration-enabled-keys";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";
import type { IntegrationEnabledControlProps } from "./integration-enabled-control-props";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

const WORKSPACE_A = "ws-a";
const WORKSPACE_B = "ws-b";

// Every slider, with the name its switch is labelled by. Rendered against the
// real hooks rather than mocks: the workspace has to survive the whole
// page → control → hook → localStorage chain, and a mock at any hop would hide
// a dropped argument.
const CONTROLS: ReadonlyArray<
  [keyof typeof INTEGRATION_ENABLED_KEYS, string, ComponentType<IntegrationEnabledControlProps>]
> = [
  ["azure-devops", "Azure DevOps", AzureDevOpsEnabledControl],
  ["github", "GitHub", GitHubEnabledControl],
  ["gitlab", "GitLab", GitLabEnabledControl],
  ["jira", "Jira", JiraEnabledControl],
  ["linear", "Linear", LinearEnabledControl],
  ["sentry", "Sentry", SentryEnabledControl],
];

describe.each(CONTROLS)("%s enable/disable slider", (slug, name, Control) => {
  const { storageKey } = INTEGRATION_ENABLED_KEYS[slug];

  beforeEach(() => localStorageMock.clear());
  afterEach(() => {
    cleanup();
    localStorageMock.clear();
  });

  it("persists to the workspace it was given", async () => {
    // Queries are scoped to this render, not the document: several of these
    // sliders can be mounted at once on the index page, and a global lookup
    // would be free to answer with another one's switch.
    const view = render(
      <SettingsSaveProvider>
        <Control workspaceId={WORKSPACE_A} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(view.getByRole("switch", { name: `Enable ${name}` }));
    fireEvent.click(view.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(window.localStorage.getItem(`${storageKey}:${WORKSPACE_A}`)).toBe("false"),
    );
    expect(window.localStorage.getItem(storageKey)).toBeNull();
  });

  it("reads the workspace it was given, not another one's state", () => {
    window.localStorage.setItem(`${storageKey}:${WORKSPACE_B}`, "false");

    const view = render(
      <SettingsSaveProvider>
        <Control workspaceId={WORKSPACE_A} />
      </SettingsSaveProvider>,
    );

    expect(view.getByRole("switch", { name: `Enable ${name}` }).getAttribute("aria-checked")).toBe(
      "true",
    );
  });
});
