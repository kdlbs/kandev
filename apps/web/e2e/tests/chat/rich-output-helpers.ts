import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

export const RICH_OUTPUT_FILE = "rich-output-report.txt";
export const RICH_OUTPUT_FILE_CONTENT = "RICH_OUTPUT_FILE_PREVIEW\nchecks=38\nfailures=0\n";
const RICH_OUTPUT_CSV = "rich-output-chart-data.csv";
const RICH_OUTPUT_CSV_CONTENT = [
  "recorded_at,route,p50_ms,p95_ms,requests,errors",
  "2026-08-12T10:00:00Z,/api,18.2,29.4,2400,12",
  "2026-08-13T10:00:00Z,/health,16.4,26.1,800,2",
  "2026-08-14T10:00:00Z,/events,15.9,24.8,1700,7",
  "",
].join("\n");

function richOutputScript(): string {
  const payload = {
    version: 1,
    title: "Release health",
    description: "Focused verification from the current workspace.",
    blocks: [
      {
        type: "metrics",
        items: [
          { label: "Passed", value: "38", detail: "Focused checks" },
          { label: "Failed", value: "0" },
          { label: "Coverage", value: "94%" },
        ],
      },
      {
        type: "chart",
        chart_type: "line",
        title: "Latency over time",
        summary: "p50 and p95 latency from the workspace CSV.",
        csv: {
          path: RICH_OUTPUT_CSV,
          x_column: "recorded_at",
          series: [{ column: "p95_ms", label: "p95" }],
        },
      },
      {
        type: "chart",
        chart_type: "bar",
        title: "Requests by route",
        summary: "Request and error counts from the same workspace CSV.",
        csv: {
          path: RICH_OUTPUT_CSV,
          x_column: "route",
          series: [
            { column: "requests", label: "Requests" },
            { column: "errors", label: "Errors" },
          ],
        },
      },
      {
        type: "file",
        path: RICH_OUTPUT_FILE,
        title: "Raw verification report",
        caption: "Workspace-backed evidence. Loaded only when requested.",
        mime_type: "text/plain",
      },
    ],
  };
  return [
    `e2e:mcp:kandev:show_rich_output_kandev(${JSON.stringify(payload)})`,
    'e2e:message("Rich output complete.")',
  ].join("\n");
}

export async function seedRichOutputTask({
  page,
  apiClient,
  seedData,
  title,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  title: string;
}): Promise<SessionPage> {
  fs.writeFileSync(path.join(seedData.repositoryPath, RICH_OUTPUT_FILE), RICH_OUTPUT_FILE_CONTENT);
  fs.writeFileSync(path.join(seedData.repositoryPath, RICH_OUTPUT_CSV), RICH_OUTPUT_CSV_CONTENT);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: richOutputScript(),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}
