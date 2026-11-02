import { expectNeutralMCPEvidence, captureNeutralMCPEvidence } from "./mcp-neutral-evidence";
import { expectMCPConnectionGeometry } from "./mcp-connection-geometry";
import { expect, type Locator, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import { openMCPComposerCustomization } from "./mcp-composer";
import type { MCPSessionApplyState } from "../../lib/types/http-mcp";
import { expectMCPSectionHierarchy } from "./mcp-customization-hierarchy";
import { expectMCPInheritanceBadges } from "./mcp-inheritance-badges";
import { expectMCPCustomizationScrolling } from "./mcp-customization-scrolling";
import { waitForHttp } from "./causal-waits";

const HEADLINES: Record<MCPSessionApplyState, string> = {
  applied: "Setup up to date",
  pending_idle: "Changes saved, waiting to apply",
  deferred_restart: "Saved for the next agent start",
  failed: "Changes could not be applied",
};

export async function checkMCPStatusClarity({
  page,
  apiClient,
  seedData,
  mobile,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile: boolean;
}) {
  const definitions = [];
  const inheritedName = "Shared documentation for the Atlas platform and infrastructure team";
  const taskTitle = "Verify inherited Atlas infrastructure MCP configuration";
  const previousOrigins: { scope: "profile" | "repository"; owner: string; ids: string[] }[] = [];
  try {
    for (const [index, display_name] of [
      inheritedName,
      "HTTP issue tracker",
      "Extra review notes",
      ...Array.from({ length: 9 }, (_, index) => `Scroll fixture ${index + 1}`),
      "ZZ final additional server",
    ].entries()) {
      definitions.push(
        await apiClient.createMCPServer(seedData.workspaceId, {
          runtime_name:
            index < 3 ? `clarity-${index}` : `clarity-extra-${String(index).padStart(2, "0")}`,
          display_name,
          description: index === 0 ? "Shared guides for the Atlas infrastructure team." : undefined,
          source: "custom",
          execution_mode: "remote",
          transport: index === 1 ? "streamable_http" : "sse",
          configuration: { url: `https://mcp.example.test/clarity-${index}` },
        }),
      );
    }
    for (const [scope, owner] of [
      ["profile", seedData.agentProfileId],
      ["repository", seedData.repositoryId],
    ] as const) {
      const previous = await apiClient.getMCPSelections(scope, owner, seedData.workspaceId);
      previousOrigins.push({ scope, owner, ids: previous.definition_ids });
      await apiClient.replaceMCPSelections(scope, owner, seedData.workspaceId, [definitions[0].id]);
    }
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      taskTitle,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        mcp_server_ids: definitions.slice(0, 2).map((item) => item.id),
      },
    );
    if (!task.session_id) throw new Error("Missing clarity session");
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.sendMessageViaButton("e2e:mcp:kandev:list_workspaces_kandev({})");
    await session.waitForChatIdle({ timeout: 30_000 });
    const trigger = page.getByTestId("mcp-status-trigger");
    if (!mobile) {
      await page.mouse.move(0, 0);
      await trigger.hover();
      const tooltip = page.getByRole("tooltip").getByTestId("mcp-status-popover");
      await expect(tooltip).toContainText("Current agent");
      await expect(tooltip.getByTestId("mcp-tooltip-server-clarity-1")).toContainText(
        "This agent does not support HTTP MCP servers.",
      );
      await expect(tooltip.getByTestId("mcp-tooltip-server-kandev")).toContainText(
        "Tools loaded for this agent.",
      );
      await expect(tooltip.getByTestId("mcp-tooltip-server-kandev")).not.toContainText(
        "does not support HTTP",
      );
      await expect(tooltip.getByTestId("mcp-tooltip-server-clarity-0")).toContainText(
        "The connection has not been verified.",
      );
      await expectNeutralMCPEvidence(tooltip.getByTestId("mcp-tooltip-server-clarity-0"));
      await checkWrappedTooltipAlignment(
        page.locator('[data-slot="tooltip-content"] > [data-testid="mcp-status-popover"]'),
      );
    }
    await trigger.click();
    const explorer = page.getByTestId("mcp-server-explorer");
    await expect(explorer.getByTestId("mcp-server-row-clarity-1")).toContainText("Not available");
    await expect(explorer.getByTestId("mcp-server-row-clarity-1")).toContainText(
      "This agent does not support HTTP MCP servers.",
    );
    await expectMCPConnectionGeometry(explorer, mobile);
    await expectNeutralMCPEvidence(explorer.getByTestId("mcp-server-row-clarity-0"));
    await explorer.getByTestId("mcp-server-row-clarity-0").click();
    await expectNeutralMCPEvidence(explorer.getByTestId("mcp-server-detail"));
    if (mobile)
      await explorer.getByRole("button", { name: "Back to servers", exact: true }).click();
    await explorer.getByTestId("mcp-server-row-clarity-1").click();
    await expect(explorer.getByTestId("mcp-server-detail")).toContainText(
      "This agent does not support HTTP MCP servers.",
    );
    if (mobile)
      await explorer.getByRole("button", { name: "Back to servers", exact: true }).click();
    await explorer.getByTestId("mcp-customize-tab").click();
    await expect(
      explorer.getByRole("heading", { name: "Selected for this task", exact: true }),
    ).toHaveCount(1);
    await expect(
      explorer.getByRole("heading", { name: "Additional servers", exact: true }),
    ).toHaveCount(1);
    await expect(explorer.getByTestId("mcp-inherited-group")).toContainText(inheritedName);
    await expectMCPSectionHierarchy(explorer, inheritedName);
    await expectMCPCustomizationScrolling(page, explorer, mobile);
    await expectMCPInheritanceBadges(
      explorer,
      definitions[0].id,
      taskTitle,
      [seedData.agentProfileId, seedData.repositoryId, task.id],
      mobile,
    );
    const search = explorer.getByRole("textbox", { name: "Search MCP servers" });
    await search.fill("Extra review");
    const checkbox = explorer.getByRole("checkbox");
    await expect(checkbox).toHaveCount(1);
    await checkbox.click();
    await expect
      .poll(
        async () =>
          (await apiClient.getMCPSelections("task_session", task.session_id!, seedData.workspaceId))
            .definition_ids,
      )
      .toEqual([definitions[2].id]);
    await expect(search).toHaveValue("Extra review");
    await expect(search).toBeVisible();
    await page.reload();
    await session.waitForLoad();
    await openMCPComposerCustomization(page);
    await explorer.getByRole("textbox", { name: "Search MCP servers" }).fill("Extra review");
    await expect(explorer.getByRole("checkbox")).toBeChecked();
    await explorer.getByTestId("mcp-explorer-close").click();

    await checkSavedReports(page, session, task.session_id, mobile);
  } finally {
    for (const previous of previousOrigins.reverse())
      await apiClient.replaceMCPSelections(
        previous.scope,
        previous.owner,
        seedData.workspaceId,
        previous.ids,
      );
    for (const definition of definitions.reverse())
      await apiClient.deleteMCPServer(seedData.workspaceId, definition.id, definition.revision);
  }
}

async function checkWrappedTooltipAlignment(tooltip: Locator) {
  const row = tooltip.getByTestId("mcp-tooltip-server-clarity-0");
  const name = row.getByText("clarity-0", { exact: true });
  const status = row.getByText("Delivered, connection unverified", { exact: true });
  const explanation = row.getByText(
    "Configuration sent to the agent. The connection has not been verified.",
    { exact: true },
  );
  // Force the same line break that long server names cause in the desktop hover.
  await tooltip.evaluate((element) => {
    element.style.width = "240px";
  });
  try {
    await expect
      .poll(async () => {
        const [nameBox, statusBox, explanationBox] = await Promise.all([
          name.boundingBox(),
          status.boundingBox(),
          explanation.boundingBox(),
        ]);
        if (!nameBox || !statusBox || !explanationBox) return false;
        return (
          statusBox.y > nameBox.y &&
          Math.abs(statusBox.x - nameBox.x) <= 1 &&
          Math.abs(explanationBox.x - nameBox.x) <= 1
        );
      })
      .toBe(true);
  } finally {
    await tooltip.evaluate((element) => element.style.removeProperty("width"));
  }
}

async function checkSavedReports(
  page: Page,
  session: SessionPage,
  sessionId: string,
  mobile: boolean,
) {
  const trigger = page.getByTestId("mcp-status-trigger");
  const explorer = page.getByTestId("mcp-server-explorer");
  // Controlled application reports cover states this mock cannot naturally emit.
  let reportState: MCPSessionApplyState = "pending_idle";
  const url = `**/api/v1/task-sessions/${sessionId}/mcp-selections*`;
  const handler = async (route: Route) => {
    if (route.request().method() !== "GET") return route.continue();
    const apply_state = reportState;
    const response = await route.fetch();
    const body = await response.json();
    body.mcp_state = {
      desired_revision: 3,
      applied_revision: apply_state === "applied" ? 3 : 1,
      apply_state,
    };
    await route.fulfill({
      response,
      json: body,
      headers: { ...response.headers(), "x-kandev-e2e-report": apply_state },
    });
  };
  await page.route(url, handler);
  try {
    for (const apply_state of ["pending_idle", "deferred_restart", "failed", "applied"] as const) {
      reportState = apply_state;
      const reportReceived = waitForHttp(
        page,
        "GET",
        new RegExp(`/api/v1/task-sessions/${sessionId}/mcp-selections$`),
        {
          // Reload can discard an older response body before it can be read.
          predicate: (response) =>
            response.ok() && response.headers()["x-kandev-e2e-report"] === apply_state,
        },
      );
      await page.reload();
      await reportReceived;
      await session.waitForLoad();
      if (!mobile) {
        await page.mouse.move(0, 0);
        await trigger.hover();
        const tooltip = page.getByRole("tooltip").getByTestId("mcp-status-popover");
        await expect(tooltip).toBeVisible();
        if (apply_state === "applied")
          await expect(tooltip.getByTestId("mcp-tooltip-selection-state")).toHaveCount(0);
        else {
          await expect(tooltip).toContainText("Saved changes");
          await expect(tooltip).toContainText(HEADLINES[apply_state]);
        }
      }
      await openMCPComposerCustomization(page);
      const summary = explorer.locator(
        '[data-testid="mcp-customization-scroll"] [data-testid="mcp-application-summary"]',
      );
      await expect(summary).toHaveAttribute("data-state", apply_state);
      await expect(summary).toContainText(HEADLINES[apply_state]);
      await expect(summary.locator('svg[aria-hidden="true"]')).toHaveCount(
        apply_state === "failed" ? 2 : 1,
      );
      await expect(explorer).not.toContainText("Desired revision");
      if (apply_state === "failed") {
        await expect(summary).toContainText("The last accepted setup is kept for comparison.");
        await expect(summary).not.toContainText("stays in use");
      } else if (apply_state !== "applied") {
        await expect(summary).toContainText("stays in use");
      }
      if (mobile) {
        await expect(page.locator('[data-slot="drawer-content"][data-state="open"]')).toHaveCount(
          1,
        );
        const close = explorer.getByTestId("mcp-explorer-close");
        expect((await close.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
          true,
        );
        const scroll = explorer.getByTestId("mcp-customization-scroll");
        await scroll.evaluate((el) => {
          el.scrollTop = el.scrollHeight;
        });
        await expect(close).toBeVisible();
      }
      await explorer.getByTestId("mcp-connections-tab").click();
      if (apply_state !== "applied") {
        await expect(explorer).toContainText("Saved changes");
        await expect(explorer).toContainText(HEADLINES[apply_state]);
      }
      await expect(explorer).toContainText("Current agent");
      // The mock reuses its MCP client after reconnect and reports only delivery
      // for the new attempt. Active evidence was checked before customization.
      await expect(explorer.getByTestId("mcp-server-row-kandev")).toContainText(
        "Configuration sent to the agent. The connection has not been verified.",
      );
      await expectNeutralMCPEvidence(explorer.getByTestId("mcp-server-row-kandev"));
      if (apply_state === "pending_idle") await captureNeutralMCPEvidence(page, mobile);
      await explorer.getByTestId("mcp-explorer-close").click();
      if (!mobile) await expect(trigger).toBeFocused();
    }
  } finally {
    await page.unroute(url, handler);
  }
}
