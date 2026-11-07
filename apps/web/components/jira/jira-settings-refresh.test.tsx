import { StrictMode } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { JiraConnectionSection } from "./jira-settings";
import type { JiraConfig } from "@/lib/types/jira";

const PROJECT_INPUT = "jira-project-input";
const AUTH_BANNER = "integration-auth-status-banner";
const REMOVE_BUTTON = "jira-delete-button";
const baseline: JiraConfig = {
  workspaceId: "workspace-a",
  siteUrl: "https://fixture.atlassian.net",
  email: "fixture@example.test",
  authMethod: "api_token",
  instanceType: "cloud",
  defaultProjectKey: "BASE",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-10-21T00:00:00Z",
  updatedAt: "2026-10-21T00:00:00Z",
};
let coordinator: ReturnType<typeof useSettingsSaveCoordinator>;
function Probe() {
  coordinator = useSettingsSaveCoordinator();
  return <output data-testid="dirty">{String(coordinator.hasDirty)}</output>;
}
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it.each(["success", "absence", "failure"] as const)(
  "keeps the acknowledged Save baseline after older background %s",
  async (outcome) => {
    vi.useFakeTimers();
    let reads = 0;
    let release!: (response: Response) => void;
    const held = new Promise<Response>((resolve) => {
      release = resolve;
    });
    const writes: unknown[] = [];
    const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
    vi.stubGlobal(
      "fetch",
      vi.fn((value: string, init?: RequestInit) => {
        const url = new URL(value, "http://localhost");
        expect(url.pathname).toBe("/api/v1/jira/config");
        expect(url.searchParams.get("workspace_id")).toBe("workspace-a");
        if (init?.method === "POST") {
          const body = JSON.parse(init.body as string);
          writes.push(body);
          return Promise.resolve(json({ ...baseline, ...body, updatedAt: "2026-10-22T10:00:00Z" }));
        }
        reads += 1;
        return reads === 1 ? Promise.resolve(json(baseline)) : held;
      }),
    );
    await act(async () => {
      render(
        <StateProvider>
          <ToastProvider>
            <SettingsSaveProvider>
              <JiraConnectionSection workspaceId="workspace-a" />
              <Probe />
            </SettingsSaveProvider>
          </ToastProvider>
        </StateProvider>,
      );
    });
    expect((screen.getByTestId(PROJECT_INPUT) as HTMLInputElement).value).toBe("BASE");
    expect(coordinator.hasDirty).toBe(false);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(90_000);
    });
    expect(reads).toBe(2);
    fireEvent.change(screen.getByTestId(PROJECT_INPUT), { target: { value: "SAVED" } });
    expect(coordinator.hasDirty).toBe(true);
    await act(async () => {
      await coordinator.saveAll();
    });
    expect(writes).toHaveLength(1);
    expect(coordinator.hasDirty).toBe(false);
    await act(async () => {
      release(
        outcome === "absence"
          ? new Response(null, { status: 204 })
          : json(
              outcome === "failure" ? { error: "Old read failed" } : baseline,
              outcome === "failure" ? 500 : 200,
            ),
      );
    });
    expect((screen.getByTestId(PROJECT_INPUT) as HTMLInputElement).value).toBe("SAVED");
    expect(coordinator.hasDirty).toBe(false);
  },
);

// @covers AC-UI-SETTINGS-MANUAL-SAVE-001.4
// The API and coordinator remain real; only HTTP settlement is controlled.
type Pending = {
  method: string;
  workspaceId: string;
  body?: Record<string, string>;
  resolve: (response: Response) => void;
};
const response = (value: unknown, status = 200) =>
  status === 204 ? new Response(null, { status }) : new Response(JSON.stringify(value), { status });
function wire() {
  vi.useFakeTimers();
  const requests: Pending[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      const url = new URL(value, "http://localhost");
      expect(url.pathname).toBe("/api/v1/jira/config");
      const pending = Promise.withResolvers<Response>();
      requests.push({
        method: init?.method ?? "GET",
        workspaceId: url.searchParams.get("workspace_id")!,
        body: init?.body ? JSON.parse(init.body as string) : undefined,
        resolve: pending.resolve,
      });
      return pending.promise;
    }),
  );
  return requests;
}
function Section({ workspaceId }: { workspaceId: string }) {
  return (
    <StateProvider>
      <ToastProvider>
        <SettingsSaveProvider>
          <JiraConnectionSection key={workspaceId} workspaceId={workspaceId} />
          <Probe />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>
  );
}
async function reply(request: Pending, data: unknown, status = 200) {
  await act(async () => request.resolve(response(data, status)));
}
async function mount(initial: JiraConfig | null = baseline) {
  const requests = wire();
  const view = render(<Section workspaceId="workspace-a" />);
  expect(requests).toHaveLength(1);
  await reply(requests[0], initial, initial ? 200 : 204);
  return { requests, view };
}
async function poll(requests: Pending[]) {
  const count = requests.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(90_000);
  });
  expect(requests).toHaveLength(count + 1);
  expect(requests.at(-1)?.method).toBe("GET");
  return requests.at(-1)!;
}
const project = () => screen.getByTestId(PROJECT_INPUT) as HTMLInputElement;
const edit = (value: string) => fireEvent.change(project(), { target: { value } });
function beginSave(requests: Pending[]) {
  let done!: ReturnType<typeof coordinator.saveAll>;
  act(() => {
    done = coordinator.saveAll();
  });
  const request = requests.at(-1)!;
  expect(request.method).toBe("POST");
  return { request, done };
}
async function ack(save: ReturnType<typeof beginSave>, data: unknown, status = 200) {
  await act(async () => {
    save.request.resolve(response(data, status));
    await save.done;
  });
}
const healthy = { ...baseline, lastCheckedAt: "2026-10-22T10:00:00Z" };

it("preserves newer edits, normalized ACK metadata, Reset and the next Save", async () => {
  const { requests } = await mount(healthy);
  const older = await poll(requests);
  edit("SUBMITTED");
  const save = beginSave(requests);
  edit("NEWER");
  const accepted = {
    ...healthy,
    siteUrl: "https://canonical.atlassian.net",
    defaultProjectKey: "CANONICAL",
    lastOk: false,
    lastError: "Saved probe result",
  };
  await ack(save, accepted);
  await reply(older, baseline);
  expect(project().value).toBe("NEWER");
  expect(coordinator.hasDirty).toBe(true);
  expect(screen.getByTestId(AUTH_BANNER).textContent).toContain("Saved probe result");
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  });
  expect(project().value).toBe("CANONICAL");
  expect((screen.getByTestId("jira-site-input") as HTMLInputElement).value).toBe(accepted.siteUrl);
  expect(coordinator.hasDirty).toBe(false);
  edit("SECOND");
  const second = beginSave(requests);
  expect(second.request.body?.defaultProjectKey).toBe("SECOND");
  expect(second.request.body?.siteUrl).toBe(accepted.siteUrl);
  expect(second.request.body).not.toHaveProperty("secret");
  await ack(second, { ...accepted, defaultProjectKey: "SECOND" });
  expect(coordinator.hasDirty).toBe(false);
});

it("seeds an unchanged form from the normalized POST response", async () => {
  const { requests } = await mount();
  edit("SUBMITTED");
  await ack(beginSave(requests), {
    ...baseline,
    siteUrl: "https://canonical.atlassian.net",
    defaultProjectKey: "CANONICAL",
  });
  expect(project().value).toBe("CANONICAL");
  expect(coordinator.hasDirty).toBe(false);
});

it.each([200, 204, 500])("keeps the latest overlapping read after an older %s", async (status) => {
  const { requests } = await mount(healthy);
  const older = await poll(requests);
  const newer = await poll(requests);
  await reply(newer, {
    ...healthy,
    defaultProjectKey: "LATEST",
    lastOk: false,
    lastError: "Fresh probe",
  });
  await reply(older, baseline, status);
  expect(screen.getByTestId(AUTH_BANNER).textContent).toContain("Fresh probe");
  expect(project().value).toBe("BASE");
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  });
  expect(project().value).toBe("LATEST");
});

it("admits fresh health and null after Save without replaying old reads", async () => {
  const { requests } = await mount(healthy);
  const older = await poll(requests);
  edit("SAVED");
  const accepted = { ...healthy, defaultProjectKey: "SAVED" };
  await ack(beginSave(requests), accepted);
  await reply(await poll(requests), { ...accepted, lastOk: false, lastError: "Fresh health" });
  await reply(older, baseline);
  expect(screen.getByTestId(AUTH_BANNER).textContent).toContain("Fresh health");
  expect(coordinator.hasDirty).toBe(false);
  await reply(await poll(requests), null, 204);
  expect(screen.queryByTestId(REMOVE_BUTTON)).toBeNull();
  expect(screen.queryByTestId(AUTH_BANNER)).toBeNull();
  expect(coordinator.hasDirty).toBe(true);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  });
  expect(project().value).toBe("");
  expect(coordinator.hasDirty).toBe(false);
});

it("fences a read started during Save and admits the successful write", async () => {
  const { requests } = await mount();
  edit("SAVED");
  const save = beginSave(requests);
  const during = await poll(requests);
  await ack(save, { ...baseline, defaultProjectKey: "SAVED" });
  await reply(during, null, 204);
  expect(project().value).toBe("SAVED");
  expect(coordinator.hasDirty).toBe(false);
});

async function remove() {
  fireEvent.click(screen.getByTestId(REMOVE_BUTTON));
  await act(async () => {
    fireEvent.click(screen.getByTestId("jira-remove-confirm"));
  });
}

it.each(["save", "remove"] as const)(
  "failed %s preserves its baseline and permits pending reads",
  async (action) => {
    const { requests } = await mount(healthy);
    const older = await poll(requests);
    if (action === "save") {
      edit("UNSAVED");
      await ack(beginSave(requests), { error: "Write rejected" }, 500);
    } else {
      await remove();
      expect(requests.at(-1)?.method).toBe("DELETE");
      await reply(requests.at(-1)!, { error: "Write rejected" }, 500);
    }
    await reply(older, { ...healthy, lastOk: false, lastError: "Read still eligible" });
    expect(screen.getByTestId(AUTH_BANNER).textContent).toContain("Read still eligible");
    expect(screen.getByTestId(REMOVE_BUTTON)).toBeTruthy();
    if (action === "save") expect(coordinator.hasDirty).toBe(true);
    else edit("LOCAL");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    });
    expect(project().value).toBe("BASE");
    expect(coordinator.hasDirty).toBe(false);
  },
);

it.each([200, 204, 500])("removal fences an older %s and permits later config", async (status) => {
  const { requests } = await mount();
  const older = await poll(requests);
  await remove();
  expect(requests.at(-1)?.method).toBe("DELETE");
  await reply(requests.at(-1)!, { deleted: true });
  await reply(older, baseline, status);
  expect(project().value).toBe("");
  expect(coordinator.hasDirty).toBe(false);
  expect(screen.queryByTestId(REMOVE_BUTTON)).toBeNull();
  await reply(await poll(requests), baseline);
  expect(screen.getByTestId(REMOVE_BUTTON)).toBeTruthy();
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  });
  expect(project().value).toBe("BASE");
});

it.each([200, 204, 500])("isolates a workspace replacement from an older %s", async (status) => {
  const { requests, view } = await mount();
  const older = await poll(requests);
  view.rerender(<Section workspaceId="workspace-b" />);
  expect(requests.at(-1)?.workspaceId).toBe("workspace-b");
  await reply(requests.at(-1)!, {
    ...baseline,
    workspaceId: "workspace-b",
    defaultProjectKey: "OTHER",
  });
  await reply(older, baseline, status);
  expect(project().value).toBe("OTHER");
  expect(coordinator.hasDirty).toBe(false);
  edit("LOCAL");
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  });
  expect(project().value).toBe("OTHER");
});

it("disposes pending reads and timers without touching a remounted owner", async () => {
  const { requests, view } = await mount();
  const older = await poll(requests);
  view.unmount();
  const count = requests.length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(90_000);
  });
  expect(requests).toHaveLength(count);
  render(<Section workspaceId="workspace-a" />);
  await reply(requests.at(-1)!, { ...baseline, defaultProjectKey: "REMOUNTED" });
  await reply(older, baseline);
  expect(project().value).toBe("REMOUNTED");
  expect(coordinator.hasDirty).toBe(false);
});

it("loads absence as a clean editable form", async () => {
  await mount(null);
  expect(project().value).toBe("");
  expect(project().disabled).toBe(false);
  expect(coordinator.hasDirty).toBe(false);
});

it("rejects a disposed StrictMode initial read and admits the active read", async () => {
  const requests = wire();
  render(
    <StrictMode>
      <Section workspaceId="workspace-a" />
    </StrictMode>,
  );
  expect(requests).toHaveLength(2);
  await reply(requests[1], { ...baseline, defaultProjectKey: "CURRENT" });
  await reply(requests[0], baseline);
  expect(project().value).toBe("CURRENT");
  expect(coordinator.hasDirty).toBe(false);
});

it("keeps initial form seeding authoritative while the first load is pending", async () => {
  const requests = wire();
  render(<Section workspaceId="workspace-a" />);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(90_000);
  });
  await reply(requests[0], baseline);
  expect(project().value).toBe("BASE");
  expect(project().disabled).toBe(false);
  expect(coordinator.hasDirty).toBe(false);
});

it("does not replay an older success when the latest refresh fails", async () => {
  const { requests } = await mount(healthy);
  const older = await poll(requests);
  const newer = await poll(requests);
  await reply(newer, { error: "Fresh read failed" }, 500);
  await reply(older, { ...healthy, defaultProjectKey: "OBSOLETE" });
  expect(project().value).toBe("BASE");
  expect(coordinator.hasDirty).toBe(false);
});
