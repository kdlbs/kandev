import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, INTERIM_SETTINGS_INTERLOCK_ERROR_CODE } from "@/lib/api/client";
import { enqueueAgentInstall } from "./agent-install-catalog";

const { installAgent } = vi.hoisted(() => ({
  installAgent: vi.fn(),
}));

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  installAgent,
}));

afterEach(() => {
  vi.clearAllMocks();
});

function handledStaleError() {
  const error = new ApiError("interim settings interlock required", 403, {
    error_code: INTERIM_SETTINGS_INTERLOCK_ERROR_CODE,
  });
  error.handled = true;
  return error;
}

describe("enqueueAgentInstall", () => {
  it("does not create a local failed job for a handled stale-page error", async () => {
    installAgent.mockRejectedValueOnce(handledStaleError());
    const upsertInstallJob = vi.fn();

    await enqueueAgentInstall("claude-acp", upsertInstallJob);

    expect(upsertInstallJob).not.toHaveBeenCalled();
  });
});
