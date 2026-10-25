import { StrictMode } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { LinearConnectionSection } from "./linear-settings";
import { i18n } from "@/lib/i18n";
import type { LinearConfig } from "@/lib/types/linear";

const OTHER_WORKSPACE = "workspace-b";
const HEALTH_STATE = "data-state";
const KEY_EXAMPLE = "lin_api_...";
const SAVED_MASK = "••••••••";
const DELETE_BUTTON = "linear-delete-button";
const COMBOBOX = "combobox";

const initial: LinearConfig = {
  workspaceId: "workspace-a",
  authMethod: "api_key",
  defaultTeamKey: "ENG",
  hasSecret: true,
  lastOk: true,
  lastCheckedAt: "2026-10-22T10:00:00Z",
  createdAt: "2026-10-22T09:00:00Z",
  updatedAt: "2026-10-22T10:00:00Z",
};
const accepted = {
  ...initial,
  defaultTeamKey: "CANON",
  lastOk: false,
  lastError: "Accepted saved probe",
  lastCheckedAt: "2026-10-22T11:00:00Z",
};
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
const absent = () => new Response(null, { status: 204 });
const failed = () => json({ error: "Confirmed transport failure" }, 500);
function oldResponse(outcome: string) {
  if (outcome === "absence") return absent();
  return outcome === "failure" ? failed() : json(initial);
}
const deferred = () => Promise.withResolvers<Response>();
let coordinator: ReturnType<typeof useSettingsSaveCoordinator>;
function Probe() {
  coordinator = useSettingsSaveCoordinator();
  return null;
}
function tree(workspace = "workspace-a", strict = false) {
  const child = <LinearConnectionSection workspaceId={workspace} />;
  const content = (
    <StateProvider>
      <ToastProvider>
        <SettingsSaveProvider>
          {child}
          <Probe />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>
  );
  return strict ? <StrictMode>{content}</StrictMode> : content;
}
const input = () => screen.getByTestId("linear-secret-input") as HTMLInputElement;
const banner = () => screen.queryByTestId("integration-auth-status-banner");
const edit = (value: string) => fireEvent.change(input(), { target: { value } });
async function tick() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(90_000);
  });
}
async function settle(held: ReturnType<typeof deferred>, response: Response) {
  await act(async () => {
    held.resolve(response);
  });
}
function save() {
  let pending!: ReturnType<typeof coordinator.saveAll>;
  act(() => {
    pending = coordinator.saveAll();
  });
  return pending;
}
async function remove() {
  fireEvent.click(screen.getByTestId(DELETE_BUTTON));
  await act(async () => {
    fireEvent.click(screen.getByTestId("linear-remove-confirm"));
  });
}
function transport() {
  vi.useFakeTimers();
  const reads: ReturnType<typeof deferred>[] = [];
  const writes: { method: string; body: unknown; response: ReturnType<typeof deferred> }[] = [];
  const teams: string[] = [];
  const probes: { body: unknown; response: ReturnType<typeof deferred> }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      const url = new URL(value, "http://localhost");
      const workspace = url.searchParams.get("workspace_id");
      expect(workspace).toMatch(/^workspace-[ab]$/);
      if (url.pathname === "/api/v1/linear/teams") {
        teams.push(workspace!);
        return Promise.resolve(
          json({
            teams: [
              { id: "1", key: "ENG", name: "Engineering" },
              { id: "2", key: "CANON", name: "Canonical" },
            ],
          }),
        );
      }
      if (url.pathname === "/api/v1/linear/config/test") {
        const response = deferred();
        probes.push({ body: JSON.parse(init!.body as string), response });
        return response.promise;
      }
      expect(url.pathname).toBe("/api/v1/linear/config");
      const response = deferred();
      if (!init?.method || init.method === "GET") reads.push(response);
      else
        writes.push({
          method: init.method,
          body: init.body ? JSON.parse(init.body as string) : null,
          response,
        });
      return response.promise;
    }),
  );
  return { reads, writes, teams, probes };
}
async function mounted() {
  const t = transport();
  const view = render(tree());
  await settle(t.reads[0], json(initial));
  expect(input().placeholder).toBe(SAVED_MASK);
  expect(coordinator.hasDirty).toBe(false);
  return { ...t, view };
}
afterEach(async () => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  await i18n.changeLanguage("en");
});

// @covers AC-UI-SETTINGS-MANUAL-SAVE-001.4
it.each(["success", "absence", "failure"])(
  "keeps acknowledged Save after old %s",
  async (outcome) => {
    const t = await mounted();
    await tick();
    expect(t.reads).toHaveLength(2);
    edit("raw-submitted-key");
    const saving = save();
    expect(t.writes[0].body).toEqual({
      authMethod: "api_key",
      defaultTeamKey: "ENG",
      secret: "raw-submitted-key",
    });
    await settle(t.writes[0].response, json(accepted));
    await saving;
    expect(coordinator.hasDirty).toBe(false);
    expect(banner()?.textContent).toContain(accepted.lastError);
    await settle(t.reads[1], oldResponse(outcome));
    expect(screen.queryByTestId(DELETE_BUTTON)).not.toBeNull();
    expect(banner()?.textContent).toContain(accepted.lastError);
    expect(input().placeholder).toBe(SAVED_MASK);
    expect(input().value).toBe("");
    expect(coordinator.hasDirty).toBe(false);
    await tick();
    await settle(t.reads[2], absent());
    expect(screen.queryByTestId(DELETE_BUTTON)).toBeNull();
    expect(coordinator.hasDirty).toBe(false);
  },
);

it.each(["success", "absence", "failure"])(
  "rejects overlapping old %s after a newer read",
  async (outcome) => {
    const t = await mounted();
    await tick();
    await tick();
    expect(t.reads).toHaveLength(3);
    await settle(t.reads[2], json(accepted));
    await settle(t.reads[1], oldResponse(outcome));
    expect(banner()?.textContent).toContain(accepted.lastError);
    await tick();
    await settle(t.reads[3], absent());
    expect(screen.queryByTestId(DELETE_BUTTON)).toBeNull();
    await tick();
    await settle(t.reads[4], json(initial));
    expect(banner()?.getAttribute(HEALTH_STATE)).toBe("ok");
  },
);

it("does not revive an older read when the newest read fails", async () => {
  const t = await mounted();
  await tick();
  await tick();
  await settle(t.reads[2], failed());
  await settle(t.reads[1], json(accepted));
  expect(banner()?.getAttribute(HEALTH_STATE)).toBe("ok");
});

it.each([false, true])(
  "keeps accepted Remove against pending reads (read during write: %s)",
  async (during) => {
    const t = await mounted();
    if (!during) await tick();
    await remove();
    expect(t.writes[0].method).toBe("DELETE");
    if (during) await tick();
    await settle(t.writes[0].response, json({ deleted: true }));
    expect(screen.queryByTestId(DELETE_BUTTON)).toBeNull();
    await settle(t.reads[1], json(initial));
    expect(screen.queryByTestId(DELETE_BUTTON)).toBeNull();
    expect(input().placeholder).toBe(KEY_EXAMPLE);
    expect(banner()).toBeNull();
    await tick();
    await settle(t.reads[2], json(initial));
    expect(screen.getByTestId(DELETE_BUTTON)).toBeTruthy();
  },
);

it("accepts Save regardless of reads admitted during the write, preserving raw draft and Reset", async () => {
  const t = await mounted();
  edit(" submitted ");
  const saving = save();
  await tick();
  edit(" newer raw ");
  await settle(t.writes[0].response, json(accepted));
  await saving;
  await settle(t.reads[1], absent());
  expect(input().value).toBe(" newer raw ");
  expect(coordinator.hasDirty).toBe(true);
  expect(banner()?.textContent).toContain(accepted.lastError);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  });
  expect(input().value).toBe("");
  expect(screen.getByRole(COMBOBOX).textContent).toContain("Canonical");
  expect(coordinator.hasDirty).toBe(false);
  edit("next raw");
  const next = save();
  expect(t.writes[1].body).toEqual({
    authMethod: "api_key",
    defaultTeamKey: "CANON",
    secret: "next raw",
  });
  await settle(t.writes[1].response, json(accepted));
  await next;
  expect(coordinator.hasDirty).toBe(false);
  expect(input().value).toBe("");
  await tick();
  await settle(t.reads[2], json(initial));
  expect(banner()?.getAttribute(HEALTH_STATE)).toBe("ok");
  expect(screen.getByRole(COMBOBOX).textContent).toContain("Canonical");
});

it.each(["POST", "DELETE"])(
  "failed %s retains draft, baseline and read eligibility",
  async (method) => {
    const t = await mounted();
    await tick();
    edit("retry raw");
    const saving = method === "POST" ? save() : await remove();
    await settle(t.writes[0].response, failed());
    await saving;
    expect(input().value).toBe("retry raw");
    expect(coordinator.hasDirty).toBe(true);
    expect(screen.getByText(/Confirmed transport failure/)).toBeTruthy();
    expect(banner()?.getAttribute(HEALTH_STATE)).toBe("ok");
    await settle(t.reads[1], json(accepted));
    expect(banner()?.textContent).toContain(accepted.lastError);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    });
    expect(screen.getByRole(COMBOBOX).textContent).toContain("Engineering");
    expect(input().value).toBe("");
  },
);

it("waits for slow initial seeding before starting the 90 second cadence", async () => {
  const t = transport();
  render(tree());
  await tick();
  await tick();
  expect(t.reads).toHaveLength(1);
  expect(input().disabled).toBe(true);
  await settle(t.reads[0], json(initial));
  expect(input().disabled).toBe(false);
  expect(screen.getByRole(COMBOBOX).textContent).toContain("Engineering");
  await tick();
  expect(t.reads).toHaveLength(2);
  await settle(t.reads[1], absent());
  expect(input().placeholder).toBe(KEY_EXAMPLE);
});

it("reports current initial load errors and still admits later health", async () => {
  const t = transport();
  render(tree());
  await settle(t.reads[0], failed());
  expect(input().disabled).toBe(false);
  expect(screen.getByText(/Failed to load Linear config/)).toBeTruthy();
  await tick();
  await settle(t.reads[1], json(initial));
  expect(input().placeholder).toBe(SAVED_MASK);
});

it("isolates workspace replacement from stale initial reads and errors", async () => {
  const t = transport();
  const view = render(tree());
  view.rerender(tree(OTHER_WORKSPACE));
  await settle(t.reads[1], json({ ...accepted, workspaceId: OTHER_WORKSPACE }));
  await settle(t.reads[0], failed());
  expect(banner()?.textContent).toContain(accepted.lastError);
  expect(screen.queryByText(/Failed to load Linear config/)).toBeNull();
  expect(input().disabled).toBe(false);
});

it("isolates pending reads and write ACK from an unmounted owner", async () => {
  const t = await mounted();
  await tick();
  edit("old key");
  const saving = save();
  t.view.unmount();
  render(tree(OTHER_WORKSPACE));
  await settle(t.reads[2], json({ ...initial, workspaceId: OTHER_WORKSPACE }));
  await settle(t.reads[1], json(accepted));
  await settle(t.writes[0].response, json(accepted));
  await saving;
  expect(banner()?.getAttribute(HEALTH_STATE)).toBe("ok");
  expect(input().value).toBe("");
  expect(coordinator.hasDirty).toBe(false);
});

it("StrictMode replay does not publish the disposed initial response or clear loading", async () => {
  const t = transport();
  render(tree("workspace-a", true));
  expect(t.reads).toHaveLength(2);
  await settle(t.reads[0], json(accepted));
  expect(input().disabled).toBe(true);
  expect(banner()).toBeNull();
  await settle(t.reads[1], json(initial));
  expect(input().disabled).toBe(false);
  expect(banner()?.getAttribute(HEALTH_STATE)).toBe("ok");
  await tick();
  expect(t.reads).toHaveLength(3);
});

it("submits the newer raw draft on next Save without Reset", async () => {
  const t = await mounted();
  edit(" submitted raw ");
  const first = save();
  edit(" newer raw ");
  await settle(t.writes[0].response, json(accepted));
  await first;
  expect(input().value).toBe(" newer raw ");
  expect(coordinator.hasDirty).toBe(true);
  const next = save();
  expect(t.writes[1].body).toEqual({
    authMethod: "api_key",
    defaultTeamKey: "ENG",
    secret: " newer raw ",
  });
  await settle(t.writes[1].response, json(accepted));
  await next;
  expect(input().value).toBe("");
  expect(coordinator.hasDirty).toBe(false);
  expect(t.teams.length).toBeGreaterThan(0);
});

it("rejects a write acknowledgement from a replaced workspace owner", async () => {
  const t = await mounted();
  edit("old workspace key");
  const saving = save();
  t.view.rerender(tree(OTHER_WORKSPACE));
  await settle(t.reads[1], json({ ...initial, workspaceId: OTHER_WORKSPACE }));
  await settle(t.writes[0].response, json(accepted));
  await saving;
  expect(banner()?.getAttribute(HEALTH_STATE)).toBe("ok");
  expect(screen.getByRole(COMBOBOX).textContent).toContain("Engineering");
  expect(input().value).toBe("");
  expect(coordinator.hasDirty).toBe(false);
});

it("keeps the latest absent read against an older successful read", async () => {
  const t = await mounted();
  await tick();
  await tick();
  await settle(t.reads[2], absent());
  await settle(t.reads[1], json(initial));
  expect(screen.queryByTestId(DELETE_BUTTON)).toBeNull();
  expect(input().placeholder).toBe(KEY_EXAMPLE);
});

it("retries a confirmed failed Save with the unchanged raw draft", async () => {
  const t = await mounted();
  edit(" retry this raw key ");
  const first = save();
  await settle(t.writes[0].response, failed());
  expect((await first).canLeave).toBe(false);
  expect(coordinator.status).toBe("error");
  expect(input().value).toBe(" retry this raw key ");
  const retry = save();
  expect(t.writes[1].body).toEqual(t.writes[0].body);
  await settle(t.writes[1].response, json(accepted));
  expect((await retry).canLeave).toBe(true);
  expect(coordinator.hasDirty).toBe(false);
  expect(input().value).toBe("");
  expect(banner()?.textContent).toContain(accepted.lastError);
});

it("keeps confirmed Remove out of a pending Save, then admits Remove", async () => {
  const t = await mounted();
  edit("save before remove");
  fireEvent.click(screen.getByTestId(DELETE_BUTTON));
  const saving = save();
  await act(async () => {
    fireEvent.click(screen.getByTestId("linear-remove-confirm"));
  });
  expect(t.writes.map((write) => write.method)).toEqual(["POST"]);
  await settle(t.writes[0].response, json(accepted));
  await saving;
  await remove();
  expect(t.writes.map((write) => write.method)).toEqual(["POST", "DELETE"]);
  await settle(t.writes[1].response, failed());
  expect(input().placeholder).toBe(SAVED_MASK);
  edit("save after failed remove");
  const next = save();
  expect(t.writes[2].method).toBe("POST");
  await settle(t.writes[2].response, json(accepted));
  await next;
  expect(coordinator.hasDirty).toBe(false);
});

it("keeps Save out of a pending Remove and releases the route gate on success", async () => {
  const t = await mounted();
  edit("pending raw draft");
  await remove();
  const blocked = save();
  expect(t.writes.map((write) => write.method)).toEqual(["DELETE"]);
  expect((await blocked).canLeave).toBe(false);
  expect(input().value).toBe("pending raw draft");
  await settle(t.writes[0].response, json({ deleted: true }));
  edit("new connection raw");
  const next = save();
  expect(t.writes[1].method).toBe("POST");
  await settle(t.writes[1].response, json(accepted));
  await next;
  expect(coordinator.hasDirty).toBe(false);
});

it.each(["save", "reset"])("preserves edits during Remove before %s", async (nextAction) => {
  const t = await mounted();
  await remove();
  edit("raw entered during Remove");
  await settle(t.writes[0].response, json({ deleted: true }));
  expect(input().value).toBe("raw entered during Remove");
  expect(input().placeholder).toBe(KEY_EXAMPLE);
  expect(coordinator.hasDirty).toBe(true);
  if (nextAction === "reset") {
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    });
    expect(input().value).toBe("");
    expect(coordinator.hasDirty).toBe(false);
    edit("raw after Reset");
  }
  const next = save();
  expect(t.writes[1].body).toMatchObject({
    secret: nextAction === "reset" ? "raw after Reset" : "raw entered during Remove",
  });
  await settle(t.writes[1].response, json(accepted));
  await next;
  expect(input().value).toBe("");
  expect(coordinator.hasDirty).toBe(false);
  expect(banner()?.textContent).toContain(accepted.lastError);
});

it("keeps initial seeding pending across a real locale change without restarting it", async () => {
  const t = transport();
  render(tree());
  await act(async () => {
    await i18n.changeLanguage("pt-pt");
  });
  expect(t.reads).toHaveLength(1);
  expect(input().disabled).toBe(true);
  await tick();
  expect(t.reads).toHaveLength(1);
  await settle(t.reads[0], json(accepted));
  expect(input().disabled).toBe(false);
  expect(input().placeholder).toBe(SAVED_MASK);
  expect(coordinator.hasDirty).toBe(false);
  expect(banner()?.textContent).toContain(accepted.lastError);
  await tick();
  expect(t.reads).toHaveLength(2);
  await settle(t.reads[1], json(initial));
});

it.each(["POST", "DELETE"])("blocks new Test probes during pending %s", async (method) => {
  const t = await mounted();
  const probe = () => fireEvent.click(screen.getByTestId("linear-test-button"));
  edit("ordinary probe raw");
  probe();
  expect(t.probes).toHaveLength(1);
  await settle(t.probes[0].response, json({ ok: true }));
  const pending = method === "POST" ? save() : await remove();
  probe();
  expect(t.probes).toHaveLength(1);
  await settle(t.writes[0].response, method === "POST" ? json(accepted) : json({ deleted: true }));
  await pending;
  edit("ordinary probe after mutation");
  probe();
  expect(t.probes).toHaveLength(2);
  await settle(t.probes[1].response, json({ ok: true }));
});

it("keeps an accepted Save authoritative across a real locale change", async () => {
  const t = await mounted();
  edit("save across locale change");
  const saving = save();
  await act(async () => {
    await i18n.changeLanguage("pt-pt");
  });
  expect(t.reads).toHaveLength(1);
  expect(input().disabled).toBe(false);
  await settle(t.writes[0].response, json(accepted));
  await saving;
  expect(banner()?.textContent).toContain(accepted.lastError);
  expect(coordinator.hasDirty).toBe(false);
  await tick();
  expect(t.reads).toHaveLength(2);
  await settle(t.reads[1], json(initial));
});

it("preserves a dirty draft and Reset baseline when the real locale changes", async () => {
  const t = await mounted();
  edit("raw draft across locale");
  await act(async () => {
    await i18n.changeLanguage("pt-pt");
  });
  const redundantRead = t.reads[1];
  if (redundantRead) await settle(redundantRead, json(accepted));
  expect(input().value).toBe("raw draft across locale");
  expect(t.reads).toHaveLength(1);
  expect(coordinator.hasDirty).toBe(true);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: i18n.t("settings:reset") }));
  });
  expect(input().value).toBe("");
  expect(coordinator.hasDirty).toBe(false);
  expect(screen.getByRole(COMBOBOX).textContent).toContain("Engineering");
});

it("reports a pending initial failure in the current locale without restarting reads", async () => {
  const t = transport();
  render(tree());
  await act(async () => {
    await i18n.changeLanguage("pt-pt");
  });
  expect(t.reads).toHaveLength(1);
  await settle(t.reads[0], failed());
  expect(input().disabled).toBe(false);
  expect(screen.getByText(/Confirmed transport failure/).textContent).toContain(
    i18n.t("linear:failedToLoadConfig", { error: "" }).trim(),
  );
});
