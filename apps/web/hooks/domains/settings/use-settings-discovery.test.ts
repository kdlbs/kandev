import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  authEnabled: false,
  state: {
    auth: {
      mode: "disabled" as "disabled" | "enabled" | "setup",
      user: null as { role: string } | null,
    },
    workspaces: {
      items: [] as Array<{ id: string; name: string }>,
    },
    settingsAgents: {
      items: [] as Array<{
        name: string;
        profiles: Array<{ id: string; name: string; agentDisplayName?: string }>;
      }>,
    },
    executors: {
      items: [] as Array<{
        type: string;
        profiles?: Array<{ id: string; name: string }>;
      }>,
    },
  },
  t: vi.fn((key: string) => key),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: mocks.t }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.authEnabled,
}));

import { useSettingsDiscovery } from "./use-settings-discovery";

describe("useSettingsDiscovery hook", () => {
  beforeEach(() => {
    mocks.authEnabled = false;
    mocks.state.auth.mode = "disabled";
    mocks.state.auth.user = null;
    mocks.state.workspaces.items = [];
    mocks.state.settingsAgents.items = [];
    mocks.state.executors.items = [];
  });

  it("follows the auth feature and mode when exposing account settings", () => {
    const { result, rerender } = renderHook(() => useSettingsDiscovery());

    expect(result.current.some((entry) => entry.groupId === "account")).toBe(false);

    mocks.authEnabled = true;
    rerender();

    expect(result.current.some((entry) => entry.groupId === "account")).toBe(false);

    mocks.state.auth.mode = "enabled";
    rerender();

    expect(result.current.some((entry) => entry.groupId === "account")).toBe(true);
  });

  it("exposes user administration only to admins", () => {
    mocks.authEnabled = true;
    mocks.state.auth.mode = "enabled";
    mocks.state.auth.user = { role: "member" };
    const { result, rerender } = renderHook(() => useSettingsDiscovery());

    expect(result.current.some((entry) => entry.id === "system-users")).toBe(false);

    mocks.state.auth.user = { role: "admin" };
    rerender();

    expect(result.current.some((entry) => entry.id === "system-users")).toBe(true);
  });

  it("propagates dynamic workspace, agent, and executor store updates", () => {
    const { result, rerender } = renderHook(() => useSettingsDiscovery());

    expect(result.current.some((entry) => entry.id.startsWith("workspace:"))).toBe(false);
    expect(result.current.some((entry) => entry.id.startsWith("agent-profile:"))).toBe(false);
    expect(result.current.some((entry) => entry.id.startsWith("executor-profile:"))).toBe(false);

    mocks.state.workspaces.items = [{ id: "workspace-1", name: "Product" }];
    mocks.state.settingsAgents.items = [
      {
        name: "codex",
        profiles: [{ id: "agent-1", name: "Careful", agentDisplayName: "Codex" }],
      },
    ];
    mocks.state.executors.items = [
      { type: "docker", profiles: [{ id: "executor-1", name: "Local Docker" }] },
    ];
    rerender();

    expect(result.current.find((entry) => entry.id === "workspace:workspace-1")?.label).toBe(
      "Product",
    );
    expect(result.current.find((entry) => entry.id === "agent-profile:agent-1")?.label).toBe(
      "Codex • Careful",
    );
    expect(result.current.find((entry) => entry.id === "executor-profile:executor-1")?.label).toBe(
      "Local Docker",
    );
  });
});
