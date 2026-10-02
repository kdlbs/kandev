import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentUpdateStatus } from "@/lib/api";
import { AgentRuntimePolicies } from "./agent-runtime-policies";

const { status } = vi.hoisted(() => ({
  status: {
    agent_name: "opencode-acp",
    display_name: "OpenCode",
    runtime_id: "native:opencode",
    management: "manual",
    managed_fallback: true,
    available: true,
    enabled: true,
    guidance_url: "https://opencode.ai/docs/cli/",
  } as AgentUpdateStatus,
}));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => true }));
vi.mock("@/hooks/domains/settings/use-agent-runtime-update-statuses", () => ({
  useAgentRuntimeUpdateStatuses: () => ({ statusByAgent: { "opencode-acp": status } }),
}));
vi.mock("./use-runtime-auto-update-policy", () => ({
  useRuntimeAutoUpdatePolicy: () => ({ draft: false, isDirty: false, setDraft: vi.fn() }),
}));
afterEach(cleanup);

describe("runtime control destinations", () => {
  it("retains vendor guidance when the separate card snapshot has no runtime control", () => {
    render(<AgentRuntimePolicies hasRuntimeControl={() => false} />);
    expect(screen.queryByRole("link", { name: "Manage fallback versions" })).toBeNull();
    expect(screen.getByRole("link", { name: "Manual update guidance" }).getAttribute("href")).toBe(
      "https://opencode.ai/docs/cli/",
    );
  });

  it("links to the installed runtime control after its snapshot becomes available", () => {
    render(<AgentRuntimePolicies hasRuntimeControl={(name) => name === "opencode-acp"} />);
    expect(
      screen.getByRole("link", { name: "Manage fallback versions" }).getAttribute("href"),
    ).toBe("#installed-agent-opencode-acp");
  });
});
