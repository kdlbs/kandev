import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

const TURN_FINISHED = "session.turn_finished";
const CLARIFICATION_REQUESTED = "session.clarification_requested";

/** Two providers so the desktop events table renders more than one column. */
export const PROVIDER_NAMES = ["Desktop", "Team channel"];

/**
 * The settings type scale is shared by the desktop table and the mobile
 * stacked list, so both specs seed the same providers. Keeping one helper stops
 * the two from drifting into asserting against different fixtures.
 */
export async function seedNotificationProviders(apiClient: ApiClient): Promise<void> {
  for (const name of PROVIDER_NAMES) {
    const response = await apiClient.rawRequest("POST", "/api/v1/notification-providers", {
      name,
      type: "local",
      events: [TURN_FINISHED, CLARIFICATION_REQUESTED],
    });
    expect(response.ok).toBe(true);
  }
}
