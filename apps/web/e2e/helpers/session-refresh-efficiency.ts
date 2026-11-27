import type { Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

export async function createLongRunningSession(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: 'e2e:delay(120000)\ne2e:message("session refresh fixture complete")',
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

export function matchesTaskSessionRead(url: string, sessionId: string): boolean {
  const parsed = new URL(url);
  return parsed.pathname === `/api/v1/task-sessions/${sessionId}`;
}

export type HeldTaskSessionRead = {
  captured: Promise<{ status: number; etag: string | null; state: string }>;
  release: () => void;
};

export async function holdNextRunningTaskSessionRead(
  page: Page,
  sessionId: string,
): Promise<HeldTaskSessionRead> {
  const urlPattern = `**/api/v1/task-sessions/${sessionId}`;
  let releaseResponse!: () => void;
  let resolveCaptured!: (value: { status: number; etag: string | null; state: string }) => void;
  let capturedOnce = false;
  const responseRelease = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  const captured = new Promise<{ status: number; etag: string | null; state: string }>(
    (resolve) => {
      resolveCaptured = resolve;
    },
  );

  await page.route(urlPattern, async (route) => {
    const headers = route.request().headers();
    delete headers["if-none-match"];
    const response = await route.fetch({ headers });
    if (capturedOnce || response.status() !== 200) {
      await route.fulfill({ response });
      return;
    }

    const body = await response.body();
    const payload = JSON.parse(body.toString("utf8")) as { session?: { state?: string } };
    const state = payload.session?.state;
    if (state !== "RUNNING") {
      await route.fulfill({ response, body });
      return;
    }

    capturedOnce = true;
    resolveCaptured({
      status: response.status(),
      etag: response.headers()["etag"] ?? null,
      state,
    });
    await responseRelease;
    await route.fulfill({ response, body });
  });

  return { captured, release: releaseResponse };
}
