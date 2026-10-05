import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentUpdateStatus } from "@/lib/api";
import { AgentRuntimePolicies } from "./agent-runtime-policies";
import { SettingsTargetProvider } from "./settings-target-provider";

const disclosureSelector = "#runtime-updates details";

const { status, snapshot } = vi.hoisted(() => ({
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
  snapshot: { statuses: {} as Record<string, AgentUpdateStatus> },
}));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => true }));
vi.mock("@/hooks/domains/settings/use-agent-runtime-update-statuses", () => ({
  useAgentRuntimeUpdateStatuses: () => ({ statusByAgent: snapshot.statuses }),
}));
vi.mock("./use-runtime-auto-update-policy", () => ({
  useRuntimeAutoUpdatePolicy: () => ({ draft: false, isDirty: false, setDraft: vi.fn() }),
}));
beforeEach(() => {
  snapshot.statuses = { "opencode-acp": status };
});
afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
});

function policies() {
  return (
    <SettingsTargetProvider>
      <AgentRuntimePolicies hasRuntimeControl={() => true} />
    </SettingsTargetProvider>
  );
}

function expand(view: ReturnType<typeof render>) {
  fireEvent.click(view.container.querySelector("#runtime-updates summary")!);
}

describe("runtime settings disclosure", () => {
  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.5
  it("starts collapsed on ordinary entry with mounted runtime content", () => {
    const view = render(policies());
    expect(view.container.querySelector<HTMLDetailsElement>(disclosureSelector)?.open).toBe(false);
    expect(screen.getByTestId("runtime-policy-opencode-acp")).toBeTruthy();
  });

  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.6
  it.each(["runtime-updates", "runtime-update-opencode-acp"])(
    "reveals an initial %s destination",
    async (target) => {
      window.history.replaceState({}, "", `/settings/agents#${target}`);
      const view = render(policies());
      await waitFor(() =>
        expect(view.container.querySelector<HTMLDetailsElement>(disclosureSelector)?.open).toBe(
          true,
        ),
      );
    },
  );

  it("reveals a same-page fragment after manual collapse", async () => {
    window.history.replaceState({}, "", "/settings/agents#runtime-updates");
    const view = render(policies());
    const details = view.container.querySelector<HTMLDetailsElement>(disclosureSelector)!;
    await waitFor(() => expect(details?.open).toBe(true));
    expand(view);
    await waitFor(() => expect(details.open).toBe(false));
    act(() => {
      window.history.replaceState({}, "", "/settings/agents#runtime-update-opencode-acp");
      window.dispatchEvent(new Event("hashchange"));
    });
    await waitFor(() => expect(details.open).toBe(true));
  });

  it("opens both disclosures when an inactive target arrives after navigation", async () => {
    snapshot.statuses = {};
    window.history.replaceState({}, "", "/settings/agents#runtime-update-opencode-acp");
    const view = render(policies());
    snapshot.statuses = { "opencode-acp": { ...status, available: false } };
    view.rerender(policies());
    await waitFor(() => {
      const disclosures = view.container.querySelectorAll<HTMLDetailsElement>(disclosureSelector);
      expect(disclosures).toHaveLength(2);
      expect([...disclosures].every((details) => details.open)).toBe(true);
    });
  });
});

describe("runtime control destinations", () => {
  it("retains vendor guidance when the separate card snapshot has no runtime control", () => {
    const view = render(<AgentRuntimePolicies hasRuntimeControl={() => false} />);
    expand(view);
    expect(screen.queryByRole("link", { name: "Manage fallback versions" })).toBeNull();
    expect(screen.getByRole("link", { name: "Manual update guidance" }).getAttribute("href")).toBe(
      "https://opencode.ai/docs/cli/",
    );
  });

  it("links to the installed runtime control after its snapshot becomes available", () => {
    const view = render(
      <AgentRuntimePolicies hasRuntimeControl={(name) => name === "opencode-acp"} />,
    );
    expand(view);
    expect(
      screen.getByRole("link", { name: "Manage fallback versions" }).getAttribute("href"),
    ).toBe("#installed-agent-opencode-acp");
  });
});
