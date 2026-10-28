import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

export const RICH_OUTPUT_FILE = "rich-output-report.txt";
export const RICH_OUTPUT_FILE_CONTENT = "RICH_OUTPUT_FILE_PREVIEW\nchecks=38\nfailures=0\n";
export const RICH_OUTPUT_IMAGE = "rich-output-image.PNG";

export function seedRichOutputImage(repositoryPath: string) {
  fs.copyFileSync(
    path.resolve(__dirname, "../../fixtures/rich-output-image.png"),
    path.join(repositoryPath, RICH_OUTPUT_IMAGE),
  );
  return {
    version: 2,
    catalog_version: 1,
    title: "Image evidence",
    spec: {
      root: "report",
      elements: {
        report: { type: "Stack", props: { direction: "vertical" }, children: ["image", "text"] },
        image: { type: "FilePreview", props: { path: RICH_OUTPUT_IMAGE }, children: [] },
        text: { type: "FilePreview", props: { path: RICH_OUTPUT_FILE }, children: [] },
      },
    },
  };
}

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
    version: 2,
    catalog_version: 1,
    title: "Release health",
    description: "Focused verification from the current workspace.",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "vertical" },
          children: ["overview", "latency", "requests", "evidence"],
        },
        overview: {
          type: "Stack",
          props: { direction: "horizontal" },
          children: ["checks", "interpretation"],
        },
        checks: { type: "Group", props: { title: "Checks" }, children: ["counts"] },
        counts: {
          type: "Metrics",
          props: {
            items: [
              { label: "Passed", value: "38", detail: "Focused checks" },
              { label: "Failed", value: "0" },
              { label: "Coverage", value: "94%" },
            ],
          },
          children: [],
        },
        interpretation: { type: "Group", props: { title: "Interpretation" }, children: ["note"] },
        note: {
          type: "Text",
          props: { text: "Focused checks passed. Charts retain accepted CSV data." },
          children: [],
        },
        latency: {
          type: "Chart",
          props: {
            chart_type: "line",
            title: "Latency over time",
            summary: "p95 latency from the workspace CSV.",
            csv: {
              path: RICH_OUTPUT_CSV,
              x_column: "recorded_at",
              series: [{ column: "p95_ms", label: "p95" }],
            },
          },
          children: [],
        },
        requests: {
          type: "Chart",
          props: {
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
          children: [],
        },
        evidence: {
          type: "FilePreview",
          props: {
            path: RICH_OUTPUT_FILE,
            title: "Raw verification report",
            caption: "Workspace-backed evidence. Loaded only when requested.",
          },
          children: [],
        },
      },
    },
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
  historical = false,
  failureRetry = false,
  payload,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  title: string;
  historical?: boolean;
  failureRetry?: boolean;
  payload?: unknown;
}): Promise<SessionPage> {
  fs.writeFileSync(path.join(seedData.repositoryPath, RICH_OUTPUT_FILE), RICH_OUTPUT_FILE_CONTENT);
  fs.writeFileSync(path.join(seedData.repositoryPath, RICH_OUTPUT_CSV), RICH_OUTPUT_CSV_CONTENT);
  let description = richOutputScript();
  if (failureRetry) description = 'e2e:message("Rich output retry fixture ready.")';
  if (payload)
    description = `e2e:mcp:kandev:show_rich_output_kandev(${JSON.stringify(payload)})\ne2e:message("Rich output complete.")`;
  if (historical) description = 'e2e:message("Historical replay fixture.")';
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (historical) {
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find((entry) => entry.id === task.session_id)?.state;
        },
        { timeout: 30_000 },
      )
      .toBe("WAITING_FOR_INPUT");
    const snapshot = {
      version: 1,
      resolved_charts: [
        {
          block_index: 0,
          labels: ["Accepted sample"],
          series: [{ label: "p95 (ms)", values: [29.4] }],
        },
      ],
    };
    const input = {
      version: 1,
      title: "Historical health",
      blocks: [
        {
          type: "chart",
          chart_type: "bar",
          title: "Historical latency",
          summary: "Stored CSV sample",
          csv: {
            path: "removed.csv",
            x_column: "time",
            series: [{ column: "ms", label: "p95 (ms)" }],
          },
        },
        { type: "metrics", items: [{ label: "Passed", value: "38" }] },
        { type: "file", path: RICH_OUTPUT_FILE },
      ],
    };
    const toolName = "mcp__kandev__show_rich_output_kandev";
    await apiClient.seedSessionMessage(task.session_id, {
      type: "tool_call",
      content: toolName,
      authorType: "agent",
      newTurn: true,
      turnStartedAt: new Date().toISOString(),
      turnCompletedAt: new Date().toISOString(),
      metadata: {
        tool_call_id: "historical-rich-output",
        title: toolName,
        status: "complete",
        normalized: {
          kind: "generic",
          generic: {
            name: "other",
            input,
            output: {
              content: [{ type: "text", text: JSON.stringify(snapshot) }],
              structuredContent: snapshot,
            },
          },
        },
      },
    });
  }
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  if (failureRetry) {
    await seedRichOutputFailureMessage(
      apiClient,
      task.session_id,
      'StatusList item "Build" has unsupported status "success".',
    );
    await seedRichOutputSuccessMessage(apiClient, task.session_id);
  }
  return session;
}

export async function seedRichOutputFailureMessage(
  apiClient: ApiClient,
  sessionId: string,
  diagnostic: string,
): Promise<void> {
  const toolName = "mcp__kandev__show_rich_output_kandev";
  await apiClient.seedSessionMessage(sessionId, {
    type: "tool_call",
    content: toolName,
    authorType: "agent",
    newTurn: true,
    turnStartedAt: new Date().toISOString(),
    turnCompletedAt: new Date().toISOString(),
    metadata: {
      tool_call_id: `rich-output-failure-${Date.now()}`,
      title: toolName,
      status: "error",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: { kind: "other", raw_input: { secret: "never display submitted arguments" } },
          output: {
            structuredContent: null,
            content: [{ type: "text", text: JSON.stringify({ message: diagnostic }) }],
          },
        },
      },
    },
  });
}

export async function seedRichOutputSuccessMessage(
  apiClient: ApiClient,
  sessionId: string,
): Promise<void> {
  const toolName = "mcp__kandev__show_rich_output_kandev";
  const payload = {
    version: 2,
    catalog_version: 1,
    title: "Release health",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "vertical" },
          children: ["checks", "chart", "evidence"],
        },
        checks: {
          type: "StatusList",
          props: {
            items: [
              {
                label: "Corrected verification",
                status: "passed",
                detail: "The corrected report remains available.",
              },
            ],
          },
          children: [],
        },
        chart: {
          type: "Chart",
          props: {
            chart_type: "bar",
            title: "Build results",
            summary: "Completed and failed checks in this report.",
            labels: ["Passed", "Failed"],
            series: [
              { label: "Checks", values: [38, 0] },
              { label: "Retries", values: [2, 0] },
            ],
          },
          children: [],
        },
        evidence: {
          type: "FilePreview",
          props: { path: RICH_OUTPUT_FILE, title: "Raw verification report" },
          children: [],
        },
      },
    },
  };
  await apiClient.seedSessionMessage(sessionId, {
    type: "tool_call",
    content: toolName,
    authorType: "agent",
    metadata: {
      tool_call_id: "rich-output-retry-success",
      title: toolName,
      status: "complete",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: { kind: "other", raw_input: payload },
          output: { content: "Rich output accepted." },
        },
      },
    },
  });
}
