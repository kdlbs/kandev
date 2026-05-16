import http from "node:http";
import type { AddressInfo } from "node:net";
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { openTaskSession, createStandardProfile } from "../../helpers/git-helper";

const MOCK_HTML = `<!DOCTYPE html>
<html>
<head><title>Mock Dev Server</title></head>
<body>
  <div id="app">
    <button id="submit-btn" role="button" aria-label="Submit form">Submit</button>
    <p class="description">A test paragraph</p>
  </div>
</body>
</html>`;

async function startMockDevServer(): Promise<{ port: number; close: () => Promise<void> }> {
  const server = http.createServer((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(MOCK_HTML);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  return {
    port,
    close: () =>
      new Promise<void>((resolve, reject) =>
        server.close((err) => (err ? reject(err) : resolve())),
      ),
  };
}

async function attachPreview(testPage: import("@playwright/test").Page, port: number) {
  const urlInput = testPage.locator('[placeholder*="localhost"], [placeholder*="3000"]').first();
  await urlInput.fill(`http://localhost:${port}`);
  await urlInput.press("Enter");
  await expect(testPage.locator('iframe[title="Preview"]')).toBeVisible({ timeout: 15_000 });
}

async function simulatePin(
  testPage: import("@playwright/test").Page,
  overrides: Record<string, unknown> = {},
) {
  await testPage.evaluate((data) => {
    window.postMessage(
      {
        source: "kandev-inspector",
        type: "annotation-added",
        payload: {
          id: `a-${Date.now()}`,
          number: 1,
          kind: "pin",
          pagePath: "/",
          comment: "make this primary color",
          element: {
            tag: "button",
            id: "submit-btn",
            role: "button",
            ariaLabel: "Submit form",
            text: "Submit",
            selector: "div#app > button#submit-btn",
          },
          ...data,
        },
      },
      "*",
    );
  }, overrides);
}

test.describe("Preview annotations", () => {
  test("inspect button is hidden when preview has no URL", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profile = await createStandardProfile(apiClient, "annot-no-url");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Annot no URL", profile.id, {
      description: "no url",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await openTaskSession(testPage, "Annot no URL");
    await expect(testPage.getByTestId("preview-inspect-button")).not.toBeVisible();
  });

  test("inspect button toggles and shows count badge as pins accumulate", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const devServer = await startMockDevServer();
    try {
      const profile = await createStandardProfile(apiClient, "annot-toggle");
      await apiClient.createTaskWithAgent(seedData.workspaceId, "Annot toggle", profile.id, {
        description: "toggle",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      await openTaskSession(testPage, "Annot toggle");
      await attachPreview(testPage, devServer.port);

      const btn = testPage.getByTestId("preview-inspect-button");
      await expect(btn).toBeVisible({ timeout: 5_000 });
      await expect(btn).toHaveAttribute("aria-pressed", "false");

      await btn.click();
      await expect(btn).toHaveAttribute("aria-pressed", "true");

      await simulatePin(testPage);
      await expect(testPage.getByTestId("preview-annotations-panel")).toBeVisible();
      await expect(testPage.getByTestId("preview-inspect-count")).toHaveText("1");

      await simulatePin(testPage, {
        id: "a-second",
        number: 2,
        element: {
          tag: "p",
          classes: "description",
          text: "A test paragraph",
          selector: "div#app > p.description",
        },
        comment: "increase font size",
      });
      await expect(testPage.getByTestId("preview-inspect-count")).toHaveText("2");
      await expect(testPage.getByTestId("preview-annotation-item")).toHaveCount(2);
    } finally {
      await devServer.close();
    }
  });

  test("copy writes formatted markdown of all annotations to clipboard", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const devServer = await startMockDevServer();
    try {
      const profile = await createStandardProfile(apiClient, "annot-copy");
      await apiClient.createTaskWithAgent(seedData.workspaceId, "Annot copy", profile.id, {
        description: "copy",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      await openTaskSession(testPage, "Annot copy");
      await attachPreview(testPage, devServer.port);

      await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
      await testPage.getByTestId("preview-inspect-button").click();
      await simulatePin(testPage);
      await expect(testPage.getByTestId("preview-annotations-panel")).toBeVisible();

      await testPage.getByTestId("preview-annotations-copy").click();
      const text = await testPage.evaluate(() => navigator.clipboard.readText());
      expect(text).toContain("Preview annotations on `/`");
      expect(text).toContain("[Pin]");
      expect(text).toContain("Comment: make this primary color");
      expect(text).toContain("Selector: `div#app > button#submit-btn`");
    } finally {
      await devServer.close();
    }
  });

  test("per-item remove and clear all work", async ({ testPage, apiClient, seedData }) => {
    const devServer = await startMockDevServer();
    try {
      const profile = await createStandardProfile(apiClient, "annot-remove");
      await apiClient.createTaskWithAgent(seedData.workspaceId, "Annot remove", profile.id, {
        description: "remove",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      await openTaskSession(testPage, "Annot remove");
      await attachPreview(testPage, devServer.port);

      await testPage.getByTestId("preview-inspect-button").click();
      await simulatePin(testPage);
      await simulatePin(testPage, { id: "a-two", number: 2 });
      await expect(testPage.getByTestId("preview-annotation-item")).toHaveCount(2);

      await testPage.getByTestId("preview-annotation-remove").first().click();
      await expect(testPage.getByTestId("preview-annotation-item")).toHaveCount(1);

      await testPage.getByTestId("preview-annotations-clear").click();
      await expect(testPage.getByTestId("preview-annotations-panel")).not.toBeVisible();
      await expect(testPage.getByTestId("preview-inspect-count")).not.toBeVisible();
    } finally {
      await devServer.close();
    }
  });

  test("inspect mode is preserved across logs-view round trip; annotations persist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const devServer = await startMockDevServer();
    try {
      const profile = await createStandardProfile(apiClient, "annot-logs");
      await apiClient.createTaskWithAgent(seedData.workspaceId, "Annot logs", profile.id, {
        description: "logs",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      await openTaskSession(testPage, "Annot logs");
      await attachPreview(testPage, devServer.port);

      await testPage.getByTestId("preview-inspect-button").click();
      await simulatePin(testPage);
      const btn = testPage.getByTestId("preview-inspect-button");
      await expect(btn).toHaveAttribute("aria-pressed", "true");

      await testPage.getByRole("button", { name: "Logs" }).click();
      await testPage.getByRole("button", { name: "Preview" }).click();

      // Implementation derives effectiveIsInspectMode = isInspectMode && previewView !== "output",
      // so the underlying inspect state survives the logs round-trip and the button re-activates.
      await expect(btn).toHaveAttribute("aria-pressed", "true");
      await expect(testPage.getByTestId("preview-inspect-count")).toHaveText("1");
    } finally {
      await devServer.close();
    }
  });

  test("port proxy response body contains the inspector script", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const devServer = await startMockDevServer();
    try {
      const profile = await createStandardProfile(apiClient, "annot-inject");
      const created = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Annot inject",
        profile.id,
        {
          description: "inject",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      await openTaskSession(testPage, "Annot inject");
      await attachPreview(testPage, devServer.port);

      // The port-proxy backend route expects a session ID, not the task ID
      // that appears in the URL.
      const sessionId = created.session_id;
      expect(sessionId).toBeTruthy();

      // page.request shares cookies with the page context; sufficient for the
      // current no-auth local backend.
      const proxyResp = await testPage.request.get(
        `${backend.baseUrl}/port-proxy/${sessionId}/${devServer.port}/`,
      );
      expect(proxyResp.status()).toBe(200);
      const body = await proxyResp.text();
      expect(body).toContain("kandev-inspector");
      expect(proxyResp.headers()["content-security-policy"]).toBeUndefined();
      expect(proxyResp.headers()["x-frame-options"]).toBeUndefined();
    } finally {
      await devServer.close();
    }
  });
});
