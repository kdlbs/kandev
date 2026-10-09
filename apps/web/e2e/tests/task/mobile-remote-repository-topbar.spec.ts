import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { seedTaskWithLinkedGitLabMRs, GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import { SessionPage } from "../../pages/session-page";

const repositoryOwner = "e2e-phone-owner";
const repositoryName =
  "agent-orchestrator-with-a-deliberately-long-name-for-phone-truncation-check";
const repositoryBrowserUrl = `https://github.com/${repositoryOwner}/${repositoryName}`;
const taskTitle = "Explain agent connections on a phone";

type WebSocketFrame = {
  action?: unknown;
  id?: unknown;
  payload?: unknown;
  type?: unknown;
};

async function addRemoteExecutorStatus(page: Page) {
  const statusRequests = new Set<string>();
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message === "string") {
        for (const part of message.split("\n")) {
          try {
            const frame = JSON.parse(part) as WebSocketFrame;
            if (
              frame.type === "request" &&
              frame.action === "task.session.status" &&
              typeof frame.id === "string"
            ) {
              statusRequests.add(frame.id);
            }
          } catch {
            // Ignore non-JSON WebSocket frames.
          }
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      if (typeof message !== "string") {
        socket.send(message);
        return;
      }
      const rewritten = message
        .split("\n")
        .map((part) => {
          try {
            const frame = JSON.parse(part) as WebSocketFrame;
            if (
              frame.type !== "response" ||
              typeof frame.id !== "string" ||
              !statusRequests.delete(frame.id) ||
              typeof frame.payload !== "object" ||
              frame.payload === null
            ) {
              return part;
            }
            return JSON.stringify({
              ...frame,
              payload: {
                ...frame.payload,
                is_remote_executor: true,
                executor_type: "sprites",
                executor_name: "Remote test runner",
                remote_name: "Remote test runner",
                remote_state: "running",
                remote_checked_at: new Date().toISOString(),
              },
            });
          } catch {
            return part;
          }
        })
        .join("\n");
      socket.send(rewritten);
    });
  });
}

async function expectCenterIsReachable(locator: ReturnType<Page["locator"]>, label: string) {
  const result = await locator.evaluate((element) => {
    const bounds = element.getBoundingClientRect();
    const hit = document.elementFromPoint(
      bounds.left + bounds.width / 2,
      bounds.top + bounds.height / 2,
    );
    return {
      receivesPointer: hit === element || (hit !== null && element.contains(hit)),
      bounds: { left: bounds.left, right: bounds.right, width: bounds.width },
      viewportWidth: document.documentElement.clientWidth,
      hit: hit
        ? {
            tagName: hit.tagName,
            testId: hit.getAttribute("data-testid"),
            className: typeof hit.className === "string" ? hit.className : "svg",
          }
        : null,
    };
  });
  expect(result.receivesPointer, `${label} center is covered: ${JSON.stringify(result)}`).toBe(
    true,
  );
}

test.describe("Mobile task topbar remote repository", () => {
  // @covers AC-UI-REMOTE-REPO-TOPBAR-001.1, AC-UI-REMOTE-REPO-TOPBAR-001.2,
  // AC-UI-REMOTE-REPO-TOPBAR-001.5
  test("keeps the repository link and task picker usable on a phone", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, taskTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repositories: [
        {
          remote_url: `${repositoryBrowserUrl}.git`,
          provider: "github",
          provider_owner: repositoryOwner,
          provider_name: repositoryName,
        },
      ],
    });

    await testPage.goto(`/t/${task.id}`);

    const repositoryLink = testPage.getByTestId("mobile-task-repository-link");
    const taskPicker = testPage.getByTestId("mobile-task-picker-trigger");
    await expect(repositoryLink).toHaveAttribute("href", repositoryBrowserUrl);
    await expect(repositoryLink).toHaveAttribute("target", "_blank");
    await expect(repositoryLink).toHaveAccessibleName(
      `GitHub repository ${repositoryOwner}/${repositoryName}`,
    );
    await expect(taskPicker).toContainText(taskTitle);

    const repositoryBox = await repositoryLink.boundingBox();
    const taskPickerBox = await taskPicker.boundingBox();
    expect(repositoryBox).not.toBeNull();
    expect(taskPickerBox).not.toBeNull();
    expect(repositoryBox!.width).toBeGreaterThanOrEqual(44);
    expect(repositoryBox!.height).toBeGreaterThanOrEqual(44);
    expect(taskPickerBox!.width).toBeGreaterThanOrEqual(44);
    await expect
      .poll(() =>
        testPage.getByTestId("mobile-task-repository-name").evaluate((node) => {
          const label = node as HTMLElement;
          return label.scrollWidth > label.clientWidth;
        }),
      )
      .toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "remote repository task topbar");

    await prCapture.screenshot("task-topbar-remote-repository-mobile", {
      caption: "Phone task topbar with a truncated repository link and the task picker.",
    });
    await testPage
      .context()
      .route("https://github.com/**", (route) =>
        route.fulfill({ status: 200, contentType: "text/html", body: "repository" }),
      );
    const popupPromise = testPage.waitForEvent("popup");
    await repositoryLink.tap();
    const popup = await popupPromise;
    await expect.poll(() => popup.url()).toBe(repositoryBrowserUrl);
    await expect(taskPicker).toBeVisible();

    await taskPicker.tap();
    await expect(testPage.getByRole("dialog", { name: "Tasks" })).toBeVisible();
  });

  test("keeps topbar actions reachable at 320px with a linked MR, forwarding, and remote executor", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await testPage.setViewportSize({ width: 320, height: 800 });
    await addRemoteExecutorStatus(testPage);
    const taskId = await seedTaskWithLinkedGitLabMRs(
      apiClient,
      {
        workspaceId: seedData.workspaceId,
        repositoryId: seedData.repositoryId,
        agentProfileId: seedData.agentProfileId,
        workflowId: seedData.workflowId,
        startStepId: seedData.startStepId,
      },
      "Narrow action-rich phone header",
      [191],
      "Narrow header MR",
    );

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.evaluate(
      ({ workspaceId, repositoryId, remoteUrl, providerHost }) => {
        const store = (
          window as unknown as {
            __KANDEV_E2E_STORE__?: {
              getState(): {
                repositories: {
                  itemsByWorkspaceId: Record<string, Array<Record<string, unknown>>>;
                };
              };
              setState(value: unknown): void;
            };
          }
        ).__KANDEV_E2E_STORE__;
        if (!store) throw new Error("The E2E app store is unavailable");
        const state = store.getState();
        const repositories = state.repositories.itemsByWorkspaceId[workspaceId] ?? [];
        let found = false;
        const updatedRepositories = repositories.map((repository) => {
          if (repository.id !== repositoryId) return repository;
          found = true;
          return {
            ...repository,
            source_type: "provider",
            remote_url: remoteUrl,
            provider: "gitlab",
            provider_host: providerHost,
            provider_owner: "platform",
            provider_name: "kandev",
          };
        });
        if (!found) throw new Error("The task repository is absent from the E2E app store");
        store.setState({
          repositories: {
            ...state.repositories,
            itemsByWorkspaceId: {
              ...state.repositories.itemsByWorkspaceId,
              [workspaceId]: updatedRepositories,
            },
          },
        });
      },
      {
        workspaceId: seedData.workspaceId,
        repositoryId: seedData.repositoryId,
        remoteUrl: `${GITLAB_HOST}/${GITLAB_PROJECT}.git`,
        providerHost: GITLAB_HOST,
      },
    );
    const repositoryLink = testPage.getByTestId("mobile-task-repository-link");
    const taskPicker = testPage.getByTestId("mobile-task-picker-trigger");
    await expect(testPage.getByTestId("mr-topbar-button")).toBeVisible();
    await expect(testPage.getByTestId("remote-executor-status-trigger")).toBeVisible();

    await taskPicker.tap();
    await expect(session.mobilePortForwardingToggle).toBeVisible();
    await expect(session.mobilePortForwardingToggle).toBeEnabled();
    await session.mobilePortForwardingToggle.tap();
    await expect(session.portForwardButton).toBeVisible();
    await expect(session.portForwardDialog).toBeVisible();
    await session.portForwardDialog.getByRole("button", { name: "Close" }).tap();
    await expect(session.portForwardDialog).toBeHidden();
    await expect(session.mobilePortForwardingToggle).toBeHidden();

    await expect(repositoryLink).toBeVisible();
    await expect(taskPicker).toContainText("Narrow action-rich phone header");
    const repositoryBox = await repositoryLink.boundingBox();
    const taskPickerBox = await taskPicker.boundingBox();
    expect(repositoryBox).not.toBeNull();
    expect(taskPickerBox).not.toBeNull();
    expect(repositoryBox!.width).toBeGreaterThanOrEqual(44);
    expect(taskPickerBox!.width).toBeGreaterThanOrEqual(44);
    await expectCenterIsReachable(repositoryLink, "Repository link");
    await expectCenterIsReachable(taskPicker, "Task picker");
    await assertNoDocumentHorizontalOverflow(testPage, "action-rich 320px remote task topbar");

    await testPage
      .context()
      .route(`${GITLAB_HOST}/**`, (route) =>
        route.fulfill({ status: 200, contentType: "text/html", body: "repository" }),
      );
    const popupPromise = testPage.waitForEvent("popup");
    await repositoryLink.tap();
    const popup = await popupPromise;
    await expect.poll(() => popup.url()).toBe(`${GITLAB_HOST}/${GITLAB_PROJECT}`);
    await popup.close();

    await taskPicker.tap();
    await expect(testPage.getByRole("dialog", { name: "Tasks" })).toBeVisible();
  });
});
