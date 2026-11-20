import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { RICH_OUTPUT_IMAGE, seedRichOutputImage } from "./rich-output-helpers";
import { expectTouchTarget } from "./rich-output-image-helpers";

const PACKAGE_TOOL = "kandev-rich-output/show_rich_output_kandev";
const SECOND_IMAGE = "package-preview-phone.png";

function packageReport() {
  return {
    version: 2,
    catalog_version: 1,
    title: "Built-in package evidence",
    spec: {
      root: "report",
      elements: {
        report: {
          type: "Stack",
          props: { direction: "vertical" },
          children: ["desktop", "phone", "table"],
        },
        desktop: {
          type: "FilePreview",
          props: { path: RICH_OUTPUT_IMAGE, title: "Desktop capture" },
          children: [],
        },
        phone: {
          type: "FilePreview",
          props: { path: SECOND_IMAGE, title: "Phone capture" },
          children: [],
        },
        table: {
          type: "Table",
          props: {
            columns: [
              { key: "file", label: "File" },
              { key: "result", label: "Result" },
            ],
            rows: [
              ["desktop.png", "Preview retained"],
              ["phone.png", "Phone retained"],
            ],
          },
          children: [],
        },
      },
    },
  };
}

async function seedActivity(apiClient: ApiClient, sessionId: string, id: string, newTurn = false) {
  return apiClient.seedSessionMessage(sessionId, {
    type: "tool_execute",
    content: id,
    authorType: "agent",
    newTurn,
    ...(newTurn ? { turnStartedAt: new Date().toISOString() } : {}),
    metadata: {
      status: "complete",
      tool_call_id: id,
      normalized: { shell_exec: { command: id, output: { exit_code: 0 } } },
    },
  });
}

function reportMetadata(name: string) {
  return {
    status: "complete",
    title: "mcp.kandev-rich-output.show_rich_output_kandev",
    tool_call_id: name,
    normalized: {
      kind: "generic",
      generic: {
        name,
        input: packageReport(),
        output: {
          structuredContent: null,
          content: [{ type: "text", text: "Rich output accepted." }],
        },
      },
    },
  };
}

export async function openBuiltinRichOutput({
  page,
  apiClient,
  seedData,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
}) {
  seedRichOutputImage(seedData.repositoryPath);
  fs.copyFileSync(
    path.join(seedData.repositoryPath, RICH_OUTPUT_IMAGE),
    path.join(seedData.repositoryPath, SECOND_IMAGE),
  );
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Built-in package rendering",
    seedData.agentProfileId,
    {
      description: 'e2e:message("Package fixture ready.")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const ws = watchWs(page);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  const foreignName = "foreign/show_rich_output_kandev";
  const delivered = ws.waitForEvent("session.message.added", {
    where: (payload) => payload.session_id === task.session_id && payload.content === foreignName,
  });
  const before = await seedActivity(apiClient, task.session_id, "package-before-one", true);
  await seedActivity(apiClient, task.session_id, "package-before-two");
  const report = await apiClient.seedSessionMessage(task.session_id, {
    type: "tool_call",
    content: PACKAGE_TOOL,
    authorType: "agent",
    metadata: reportMetadata(PACKAGE_TOOL),
  });
  const after = await seedActivity(apiClient, task.session_id, "package-after-one");
  await seedActivity(apiClient, task.session_id, "package-after-two");
  await apiClient.seedSessionMessage(task.session_id, {
    type: "tool_call",
    content: foreignName,
    authorType: "agent",
    metadata: reportMetadata(foreignName),
  });
  await delivered;
  if (before.turnId) await apiClient.completeSessionTurn(before.turnId);
  return {
    session,
    ids: { before: before.messageId, report: report.messageId, after: after.messageId },
  };
}

export async function expectBuiltinRichOutput(
  page: Page,
  fixture: Awaited<ReturnType<typeof openBuiltinRichOutput>>,
  mobile = false,
) {
  const chat = fixture.session.activeChat();
  const report = chat.getByTestId("rich-output");
  await expect(report).toHaveCount(1);
  await expect(report).toContainText("Built-in package evidence");
  const cards = report.getByTestId("rich-output-file");
  await expect(cards).toHaveCount(2);
  for (const title of ["Desktop capture", "Phone capture"]) {
    const card = cards.filter({ hasText: title });
    await expect(card).toBeVisible();
    const image = card.locator("img");
    await expect(image).toBeVisible();
    await expect
      .poll(() => image.evaluate((img: HTMLImageElement) => img.naturalWidth))
      .toBeGreaterThan(0);
    if (mobile) await expectTouchTarget(card.getByTestId("rich-output-file-preview-toggle"));
  }
  const table = report.getByTestId("rich-output-table");
  await expect(table).toContainText("Preview retained");
  await expect(table).toContainText("Phone retained");
  if (mobile) await expect(table.getByTestId("rich-output-table-phone")).toBeVisible();
  const selectors = [
    `#msg-turn-group-${fixture.ids.before}`,
    `#msg-${fixture.ids.report}`,
    `#msg-turn-group-${fixture.ids.after}`,
  ];
  for (const selector of selectors) await expect(chat.locator(selector)).toHaveCount(1);
  expect(
    await chat.evaluate((root, ids) => {
      const [before, reportRow, after] = ids.map((id) => root.querySelector(id)!);
      return (
        Boolean(before.compareDocumentPosition(reportRow) & Node.DOCUMENT_POSITION_FOLLOWING) &&
        Boolean(reportRow.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING)
      );
    }, selectors),
  ).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}
