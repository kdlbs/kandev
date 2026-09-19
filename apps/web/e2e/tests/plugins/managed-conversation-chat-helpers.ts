import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { installFixturePlugin } from "../../helpers/plugin-fixture";

export async function openManagedConversationChat(page: Page) {
  await installFixturePlugin(page);
  await page.goto("/plugins/e2e-managed-chat");
  await expect(page.getByTestId("managed-chat-page")).toBeVisible({ timeout: 15_000 });
  const actionResponsePromise = page.waitForResponse((response) =>
    response.url().includes("/actions/managed-chat.ensure"),
  );
  const descriptorResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" && response.url().includes("/conversation/managed/"),
  );
  await page.getByTestId("managed-chat-ensure").click();
  const actionResponse = await actionResponsePromise;
  expect(actionResponse.ok(), await actionResponse.text()).toBe(true);
  const descriptorResponse = await descriptorResponsePromise;
  expect(descriptorResponse.ok(), await descriptorResponse.text()).toBe(true);
  await expect(page.getByTestId("managed-chat-public-status")).toHaveText("ready", {
    timeout: 30_000,
  });
  await expect(page.getByTestId("workspace-agent-chat")).toBeVisible();
}

export async function exerciseManagedConversationChat(
  page: Page,
  prCapture: PrAssetCapture,
  viewport: "desktop" | "mobile",
) {
  const message = `${viewport} managed conversation message`;
  const input = page.getByLabel("Message");
  await input.fill(message);
  await page.getByRole("button", { name: "Send" }).click();
  await expect(input).toHaveValue("");
  await expect(page.getByText(message)).toBeVisible({ timeout: 30_000 });
  await prCapture.screenshot(`${viewport}-managed-conversation-ready`, {
    caption: `${viewport} managed conversation ready state using the real scoped transcript and dispatch bridge`,
  });

  await page.getByTestId("managed-chat-show-missing").click();
  await expect(page.getByTestId("workspace-agent-chat-status")).toHaveAttribute(
    "data-status",
    "deleted",
  );
  await prCapture.screenshot(`${viewport}-managed-conversation-error`, {
    caption: `${viewport} managed conversation terminal state after real descriptor resolution returns not found`,
  });
}
