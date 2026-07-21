import { expect, test } from "../../fixtures/test-base";

const MOCK_STATE = {
  authenticated: true,
  user: { ok: true, id: "user-1", displayName: "Ada Reviewer", email: "ada@example.com" },
  projects: [{ id: "project-1", name: "Platform", url: "https://dev.azure.com/acme/Platform" }],
  repositories: [
    {
      id: "azure-repo-1",
      name: "api",
      projectId: "project-1",
      projectName: "Platform",
      defaultBranch: "refs/heads/main",
      webUrl: "https://dev.azure.com/acme/Platform/_git/api",
    },
  ],
  workItems: [
    {
      id: 101,
      revision: 3,
      title: "Handle token rotation",
      state: "Active",
      type: "User Story",
      project: "project-1",
      assignedTo: "Ada Reviewer",
      webUrl: "https://dev.azure.com/acme/Platform/_workitems/edit/101",
    },
  ],
  pullRequests: [
    {
      id: 42,
      title: "Rotate integration credentials",
      status: "active",
      isDraft: false,
      sourceBranch: "refs/heads/feature/rotation",
      targetBranch: "refs/heads/main",
      author: { id: "user-1", displayName: "Ada Reviewer" },
      projectId: "project-1",
      projectName: "Platform",
      repositoryId: "azure-repo-1",
      repositoryName: "api",
      webUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/42",
      apiUrl: "https://dev.azure.com/acme/Platform/_apis/git/pullrequests/42",
    },
  ],
  feedback: {
    "42": {
      pullRequest: {
        id: 42,
        title: "Rotate integration credentials",
        status: "active",
        isDraft: false,
        sourceBranch: "refs/heads/feature/rotation",
        targetBranch: "refs/heads/main",
        author: { id: "user-1", displayName: "Ada Reviewer" },
        projectId: "project-1",
        projectName: "Platform",
        repositoryId: "azure-repo-1",
        repositoryName: "api",
        webUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/42",
        apiUrl: "https://dev.azure.com/acme/Platform/_apis/git/pullrequests/42",
      },
      reviewers: [
        {
          id: "user-2",
          displayName: "Grace Reviewer",
          vote: 10,
          isRequired: true,
          hasDeclined: false,
        },
      ],
      threads: [],
      linkedWorkItems: [{ id: 101, url: "https://dev.azure.com/acme/_apis/wit/workitems/101" }],
      policies: [{ id: "policy-1", status: "approved", name: "Build", isBlocking: true }],
      reviewState: "approved",
      policyState: "success",
    },
  },
};

test("connects and browses Azure work items, PRs, and feedback", async ({
  apiClient,
  seedData,
  testPage,
}) => {
  await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
  await testPage.goto(
    `/settings/workspace/${encodeURIComponent(seedData.workspaceId)}/integrations/azure-devops`,
  );

  const projectInput = testPage.locator("#azure-devops-project");
  const patInput = testPage.getByTestId("azure-devops-pat");
  await expect(projectInput).toBeVisible();
  const [projectBox, patBox] = await Promise.all([
    projectInput.boundingBox(),
    patInput.boundingBox(),
  ]);
  expect(projectBox).not.toBeNull();
  expect(patBox).not.toBeNull();
  expect(Math.abs(projectBox!.y - patBox!.y)).toBeLessThanOrEqual(1);
  expect(projectBox!.height).toBe(patBox!.height);

  await testPage.getByTestId("azure-devops-organization").fill("https://dev.azure.com/acme");
  await testPage.getByRole("button", { name: "How to create a personal access token" }).hover();
  const createTokenLink = testPage.getByTestId("azure-devops-pat-help").locator(":scope > a");
  await expect(createTokenLink).toHaveAttribute(
    "href",
    "https://dev.azure.com/acme/_usersSettings/tokens",
  );
  await expect(testPage.getByTestId("azure-devops-pat-help")).toContainText(
    "Work Items, check Read",
  );
  await expect(testPage.getByTestId("azure-devops-pat-help")).toContainText("Code, check Read");
  await testPage.getByTestId("azure-devops-pat").fill("azure-test-pat");
  await testPage.getByTestId("azure-devops-test-button").click();
  await expect(testPage.getByTestId("azure-devops-test-result")).toContainText(
    "Connected as Ada Reviewer",
  );
  await testPage.getByTestId("azure-devops-save-button").click();

  await testPage.goto("/azure-devops");
  await expect(testPage.getByText("Handle token rotation")).toBeVisible();

  await testPage.getByRole("button", { name: "Pull requests" }).click();
  await testPage.getByTestId("azure-devops-search-button").click();
  await expect(testPage.getByText("Rotate integration credentials")).toBeVisible();
  await testPage.getByRole("button", { name: "Feedback" }).click();
  await expect(testPage.getByTestId("azure-devops-feedback-detail")).toContainText(
    "Grace Reviewer",
  );
  await expect(testPage.getByTestId("azure-devops-feedback-detail")).toContainText("Build");
});
