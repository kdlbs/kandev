import { act, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, INTERIM_SETTINGS_INTERLOCK_ERROR_CODE } from "@/lib/api/client";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import {
  A,
  B,
  accept,
  accepted,
  cleanupCards,
  mountCards,
  owner,
  profileEvent,
  profileWire,
  publish,
  startSave,
  trigger,
} from "./custom-tui-mcp-card.test-helpers";

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

beforeEach(() => vi.useFakeTimers());
afterEach(cleanupCards);

describe("custom TUI MCP acknowledgement profiles", () => {
  // @covers AC-AGENTS-CUSTOM-ACP-001.8
  it("retains profile creation received while strategy save is pending", async () => {
    const fixture = await mountCards();
    await startSave(fixture);
    profileEvent(
      fixture,
      "created",
      profileWire(fixture.profileIds.received, A, "Received profile"),
    );
    expect(screen.getByRole("link", { name: /Received profile/ })).toBeTruthy();
    const options = fixture.store.getState().agentProfiles.items;

    await accept(fixture);

    expect(screen.queryByRole("link", { name: /Received profile/ })).not.toBeNull();
    expect(trigger().textContent).toContain("claude");
    expect(fixture.store.getState().agentProfiles.items).toEqual(options);
  });

  // @covers AC-AGENTS-CUSTOM-ACP-001.8
  it.each([
    ["updated", A],
    ["deleted", A],
    ["updated", B],
    ["deleted", B],
  ] as const)("preserves a real profile %s event for %s", async (action, id) => {
    const fixture = await mountCards();
    await startSave(fixture);
    const profileId = id === A ? fixture.profileIds.a : fixture.profileIds.b;
    const wire = { ...profileWire(profileId, id, "Current profile"), model: "current-model" };
    profileEvent(fixture, action, wire);
    const current = fixture.store.getState().settingsAgents.items.find((agent) => agent.id === id)!;
    if (action === "updated") {
      expect(current.profiles[0]).toMatchObject({
        name: "Current profile",
        model: "current-model",
      });
      expect(screen.getByRole("link", { name: /Current profile/ })).toBeTruthy();
    } else {
      expect(current.profiles).toEqual([]);
      expect(screen.queryByRole("link", { name: new RegExp(`${id} profile`) })).toBeNull();
    }
    const options = fixture.store.getState().agentProfiles.items;
    await accept(fixture);
    expect(
      fixture.store.getState().settingsAgents.items.find((agent) => agent.id === id)!.profiles,
    ).toEqual(current.profiles);
    expect(fixture.store.getState().agentProfiles.items).toEqual(options);
    if (action === "updated")
      expect(screen.getByRole("link", { name: /Current profile/ })).toBeTruthy();
    else expect(screen.queryByRole("link", { name: new RegExp(`${id} profile`) })).toBeNull();
  });
});

describe("custom TUI MCP acknowledgement membership", () => {
  it("preserves mixed membership and target metadata while accepting only owned fields", async () => {
    const fixture = await mountCards();
    await startSave(fixture);
    const currentA = {
      ...fixture.initial[0],
      name: "current-terminal",
      capability_status: "not_installed" as const,
      capability_error: "current-probe-error",
      inference_capable: true,
      mcp_config_path: "current-path",
      updated_at: "2026-10-09T00:02:00Z",
      tui_config: {
        ...fixture.initial[0].tui_config!,
        command: "current-command",
        display_name: "Current terminal",
        description: "current-description",
        disable_bracketed_paste: true,
      },
    };
    const added = owner("added-terminal", `${fixture.profileIds.received}-added`);
    publish(fixture, [added, currentA]);
    expect(screen.queryByTestId(`owner-${B}`)).toBeNull();
    expect(screen.getByTestId("owner-added-terminal")).toBeTruthy();
    await accept(fixture);
    expect(fixture.store.getState().settingsAgents.items).toEqual([
      added,
      {
        ...currentA,
        supports_mcp: true,
        tui_config: { ...currentA.tui_config, mcp_strategy: "claude" },
      },
    ]);
    expect(screen.queryByTestId(`owner-${B}`)).toBeNull();
    expect(screen.getByTestId("owner-added-terminal")).toBeTruthy();
    expect(trigger().textContent).toContain("claude");
  });
});

describe("custom TUI MCP acknowledgement identity", () => {
  // @covers AC-AGENTS-CUSTOM-ACP-001.9
  it.each([
    [A, B],
    [B, A],
  ] as const)(
    "retains both sibling strategies when %s accepts before %s",
    async (first, second) => {
      const fixture = await mountCards();
      await startSave(fixture, A, "claude");
      await startSave(fixture, B, "codex");
      const keys = { [A]: "claude", [B]: "codex" };
      await accept(fixture, accepted(fixture, first, keys[first]), first);
      expect(trigger(first).textContent).toContain(keys[first]);
      expect(trigger(first).getAttribute("disabled")).toBeNull();
      expect(trigger(second).getAttribute("disabled")).not.toBeNull();
      await accept(fixture, accepted(fixture, second, keys[second]), second);
      expect(trigger(A).textContent).toContain("claude");
      expect(trigger(B).textContent).toContain("codex");
      expect(
        fixture.store
          .getState()
          .settingsAgents.items.map((agent) => [
            agent.id,
            agent.supports_mcp,
            agent.tui_config?.mcp_strategy,
          ]),
      ).toEqual([
        [A, true, "claude"],
        [B, true, "codex"],
      ]);
    },
  );

  it("does not restore a saving target removed before its acknowledgement", async () => {
    const fixture = await mountCards();
    await startSave(fixture);
    publish(fixture, [fixture.initial[1]]);
    expect(screen.queryByTestId(`owner-${A}`)).toBeNull();
    await accept(fixture);
    expect(fixture.store.getState().settingsAgents.items).toEqual([fixture.initial[1]]);
    expect(screen.queryByTestId(`owner-${A}`)).toBeNull();
  });

  it.each(["acp", "built-in"])(
    "ignores the ACK when the current target is now %s",
    async (kind) => {
      const fixture = await mountCards();
      await startSave(fixture);
      const current = [
        {
          ...fixture.initial[0],
          tui_config:
            kind === "acp" ? { ...fixture.initial[0].tui_config!, protocol: "acp" as const } : null,
        },
        fixture.initial[1],
      ];
      publish(fixture, current);
      expect(screen.queryByTestId(`agent-mcp-strategy-${A}`)).toBeNull();
      await accept(fixture);
      expect(fixture.store.getState().settingsAgents.items).toEqual(current);
    },
  );

  it("does not apply a response belonging to a sibling request target", async () => {
    const fixture = await mountCards();
    await startSave(fixture);
    await accept(fixture, accepted(fixture, B, "codex"));
    expect(fixture.store.getState().settingsAgents.items).toEqual(fixture.initial);
    expect(trigger().getAttribute("disabled")).toBeNull();
  });
});

describe("custom TUI MCP acknowledgement controls", () => {
  // @covers AC-AGENTS-CUSTOM-ACP-001.8 AC-AGENTS-CUSTOM-ACP-001.10
  it("shows the accepted strategy without a WS echo, rather than the requested key", async () => {
    const fixture = await mountCards();
    await startSave(fixture, A, "claude");
    await accept(fixture, accepted(fixture, A, "codex"));
    expect(trigger().textContent).toContain("codex");
    expect(trigger().getAttribute("disabled")).toBeNull();
    expect(fixture.store.getState().settingsAgents.items[0].supports_mcp).toBe(true);
  });

  it.each(["empty", "omitted"])(
    "supports turning injection off with an %s accepted key",
    async (shape) => {
      const fixture = await mountCards();
      publish(fixture, [
        {
          ...fixture.initial[0],
          supports_mcp: true,
          tui_config: { ...fixture.initial[0].tui_config!, mcp_strategy: "claude" },
        },
        fixture.initial[1],
      ]);
      await startSave(fixture, A, "");
      const response = accepted(fixture, A, "");
      const omitted = { ...response.tui_config };
      Reflect.deleteProperty(omitted, "mcp_strategy");
      await accept(fixture, shape === "omitted" ? { ...response, tui_config: omitted } : response);
      expect(trigger().textContent).toContain("Off:");
      expect(trigger().getAttribute("disabled")).toBeNull();
      expect(fixture.store.getState().settingsAgents.items[0].supports_mcp).toBe(false);
    },
  );

  // @covers AC-AGENTS-CUSTOM-ACP-001.10
  it.each([false, true])(
    "preserves live changes and settles a rejected save (handled=%s)",
    async (handled) => {
      const fixture = await mountCards();
      await startSave(fixture);
      profileEvent(
        fixture,
        "created",
        profileWire(fixture.profileIds.received, A, "Retained after failure"),
      );
      expect(screen.getByRole("link", { name: /Retained after failure/ })).toBeTruthy();
      const current = fixture.store.getState().settingsAgents.items;
      const error = new ApiError("Strategy request rejected", 403, {
        error_code: INTERIM_SETTINGS_INTERLOCK_ERROR_CODE,
      });
      error.handled = handled;
      await act(async () => {
        fixture.pending.get(A)!.response.reject(error);
      });
      expect(fixture.store.getState().settingsAgents.items).toEqual(current);
      expect(
        fixture.store
          .getState()
          .settingsAgents.items[0].profiles.find(
            (profile) => profile.id === fixture.profileIds.received,
          ),
      ).toEqual(
        normalizeAgentProfile(
          profileWire(fixture.profileIds.received, A, "Retained after failure"),
        ),
      );
      expect(trigger().getAttribute("disabled")).toBeNull();
      expect(trigger().textContent).toContain("Off:");
      expect(screen.queryByText(error.message) !== null).toBe(!handled);
    },
  );
});
