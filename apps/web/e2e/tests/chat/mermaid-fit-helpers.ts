import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const WIDE_MERMAID_MESSAGE = [
  "Wide diagram:",
  "",
  "```mermaid",
  "sequenceDiagram",
  "  participant Runner as Runner (in-job)",
  "  participant Collector as OTel Collector (internal)",
  "  participant Poller as Poller (cron, outbound)",
  "  participant GitHub as GitHub Actions API",
  "  participant Tempo as Tempo Trace Store",
  "  Runner->>Collector: OTLP push lock/determinator spans (live, trace=hash(run_id))",
  "  Poller->>GitHub: GET /actions/runs + /jobs (outbound polling)",
  "  GitHub-->>Poller: run/job/step timestamps",
  "  Poller->>Collector: OTLP emit workflow+job+step spans (same trace=hash(run_id))",
  "  Collector->>Tempo: forward both sources",
  "```",
].join("\n");

export async function seedTaskWithWideMermaidMessage(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await apiClient.seedSessionMessage(task.session_id, {
    type: "message",
    content: WIDE_MERMAID_MESSAGE,
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return session;
}

export async function expectMermaidDiagramFitsViewport(session: SessionPage): Promise<void> {
  const block = session.activeChat().locator(".mermaid-block").last();
  await expect(block).toBeVisible({ timeout: 30_000 });
  await expect(block.locator(".mermaid-scroll-region svg")).toBeVisible({ timeout: 30_000 });

  await expect
    .poll(
      async () =>
        block.evaluate((element) => {
          const scrollRegion = element.querySelector<HTMLElement>(".mermaid-scroll-region");
          if (!scrollRegion) return false;
          return scrollRegion.scrollWidth <= scrollRegion.clientWidth + 1;
        }),
      { timeout: 10_000 },
    )
    .toBe(true);
}
