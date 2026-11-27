// Coordinator phase 3.1 flag contract (docs/decisions/2026-09-30-coordinator-phase-3-1-record-and-measure.md,
// "Flag boundary"). The turn ledger ships a recording half and a reading half; the reading
// half is gated on features.coordinatorPhase31, off in every shipped profile.
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

async function readFeatures(apiClient: ApiClient): Promise<Record<string, unknown>> {
  const response = await apiClient.rawRequest("GET", "/api/v1/features");
  expect(response.ok).toBe(true);
  return (await response.json()) as Record<string, unknown>;
}

test.describe("Coordinator turn ledger flag", () => {
  test("coordinatorPhase31 is off by default and follows the runtime flag", async ({
    apiClient,
    backend,
  }) => {
    const before = await readFeatures(apiClient);
    expect(before.coordinatorPhase31).toBe(false);

    const release = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR: "true",
      KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
      KANDEV_FEATURES_COORDINATOR_PHASE3: "true",
      KANDEV_FEATURES_COORDINATOR_PHASE31: "true",
    });
    try {
      const on = await readFeatures(apiClient);
      expect(on.coordinatorPhase31).toBe(true);
    } finally {
      await release();
    }
    const after = await readFeatures(apiClient);
    expect(after.coordinatorPhase31).toBe(false);
  });

  test("enabling the recording flags alone leaves the reading flag off", async ({
    apiClient,
    backend,
  }) => {
    const release = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR: "true",
      KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
    });
    try {
      const features = await readFeatures(apiClient);
      expect(features.coordinator).toBe(true);
      expect(features.coordinatorPhase2).toBe(true);
      expect(features.coordinatorPhase31).toBe(false);
    } finally {
      await release();
    }
  });
});
