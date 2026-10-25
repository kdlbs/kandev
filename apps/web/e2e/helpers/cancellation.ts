import { expect } from "@playwright/test";
import { SessionPage } from "../pages/session-page";

export const CANCEL_SETTLE_TIMEOUT_MS = 2_000;

export async function expectCancelToSettlePromptly(session: SessionPage) {
  const cancelButton = session.activeChat().getByTestId("cancel-agent-button");
  await Promise.all([
    expect(cancelButton).not.toBeVisible({ timeout: CANCEL_SETTLE_TIMEOUT_MS }),
    expect(session.idleInput()).toBeVisible({ timeout: CANCEL_SETTLE_TIMEOUT_MS }),
  ]);
}
