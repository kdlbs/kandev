import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, INTERIM_SETTINGS_INTERLOCK_ERROR_CODE } from "@/lib/api/client";
import { canInstallAgent } from "@/components/settings/install-agent-card";
import type { AvailableAgent } from "@/lib/types/http-agents";
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
  it("accepts a local-install agent with no install script", () => {
    const agent = { install_script: "", supports_local_install: true } as AvailableAgent;
    expect(canInstallAgent(agent)).toBe(true);
  });

  it("does not create a local failed job for a handled stale-page error", async () => {
    installAgent.mockRejectedValueOnce(handledStaleError());
    const upsertInstallJob = vi.fn();

    await enqueueAgentInstall("claude-acp", upsertInstallJob);

    expect(upsertInstallJob).not.toHaveBeenCalled();
  });
});
