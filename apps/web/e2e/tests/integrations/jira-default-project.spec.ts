import { test, expect } from "../../fixtures/test-base";

// Covers the "default project" preference (issue #1588 follow-up): when the
// workspace Jira config carries a defaultProjectKey, opening /jira must land on
// that project pre-selected in the Project filter (which in turn enables the
// Status filter), instead of the historical "no project" default.
test.describe("Jira default project", () => {
  const PROJECT_KEY = "CLIP";
  const OTHER_KEY = "OPS";

  async function seedProjectsAndTickets(apiClient: {
    mockJiraSetProjects: (p: { id: string; key: string; name: string }[]) => Promise<void>;
    mockJiraSetProjectStatuses: (
      key: string,
      s: { id: string; name: string; statusCategory: string }[],
    ) => Promise<void>;
    mockJiraSetSearchHits: (
      t: {
        key: string;
        summary: string;
        projectKey: string;
        statusName: string;
        statusCategory: string;
        url: string;
      }[],
    ) => Promise<void>;
  }): Promise<void> {
    await apiClient.mockJiraSetProjects([
      { id: "1", key: PROJECT_KEY, name: "Clip" },
      { id: "2", key: OTHER_KEY, name: "Ops" },
    ]);
    await apiClient.mockJiraSetProjectStatuses(PROJECT_KEY, [
      { id: "10001", name: "In Development", statusCategory: "indeterminate" },
    ]);
    await apiClient.mockJiraSetSearchHits([
      {
        key: "CLIP-1",
        summary: "Dev ticket",
        projectKey: PROJECT_KEY,
        statusName: "In Development",
        statusCategory: "indeterminate",
        url: "https://acme.atlassian.net/browse/CLIP-1",
      },
      {
        key: "OPS-9",
        summary: "Ops ticket",
        projectKey: OTHER_KEY,
        statusName: "In Development",
        statusCategory: "indeterminate",
        url: "https://acme.atlassian.net/browse/OPS-9",
      },
    ]);
  }

  test("no default project keeps the project filter unselected", async ({
    apiClient,
    testPage,
  }) => {
    await apiClient.mockJiraReset();
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      secret: "api-token-value",
    });
    await apiClient.waitForIntegrationAuthHealthy("jira");
    await seedProjectsAndTickets(apiClient);

    await testPage.goto("/jira");
    // Project pill shows no summary, and the status pill stays disabled.
    await expect(testPage.getByTestId("jira-filter-pill-project")).not.toContainText(PROJECT_KEY);
    await expect(testPage.getByTestId("jira-filter-pill-status")).toHaveAttribute(
      "data-disabled",
      "true",
    );
  });

  test("default project is pre-selected on load and narrows the list", async ({
    apiClient,
    testPage,
  }) => {
    await apiClient.mockJiraReset();
    await apiClient.setJiraConfig({
      siteUrl: "https://acme.atlassian.net",
      email: "alice@example.com",
      defaultProjectKey: PROJECT_KEY,
      secret: "api-token-value",
    });
    await apiClient.waitForIntegrationAuthHealthy("jira");
    await seedProjectsAndTickets(apiClient);

    await testPage.goto("/jira");

    // The Project pill reflects the default selection…
    await expect(testPage.getByTestId("jira-filter-pill-project")).toContainText(PROJECT_KEY);
    // …which enables the Status pill (statuses come from the selected project).
    await expect(testPage.getByTestId("jira-filter-pill-status")).not.toHaveAttribute(
      "data-disabled",
      "true",
    );
    // …and the list is narrowed to the default project's ticket only.
    await expect(testPage.getByText("CLIP-1")).toBeVisible();
    await expect(testPage.getByText("OPS-9")).toHaveCount(0);
  });
});
