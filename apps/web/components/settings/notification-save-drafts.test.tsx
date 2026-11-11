import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { NotificationProvider } from "@/lib/types/http";
import { NotificationsSettings } from "./notifications-settings";
import { SettingsSaveProvider } from "./settings-save-provider";
import {
  useIsDirty,
  useNotificationsActions,
  useNotificationsState,
  useSaveRequest,
} from "./notifications-settings-actions";

const SUBMITTED_RAW_NAME = " Submitted ";
const SUBMITTED_NAME = "Submitted";
const SUBMITTED_URL = "json://submitted";
const LATER_NAME = "Later name";
const LATER_URL = "json://later";
const SECOND_URL = "json://second";
const FIRST_URL = "json://first";
const SAVE_LABEL = "Save changes";
const CHECKED_ATTRIBUTE = "aria-checked";
const TURN = "session.turn_finished";
const ASK = "session.clarification_requested";
const UPDATE = "system.update_available";
const local: NotificationProvider = {
  id: "local-provider",
  name: "Local",
  type: "local",
  config: {},
  enabled: true,
  events: [],
  created_at: "",
  updated_at: "",
};
const apprise: NotificationProvider = {
  ...local,
  id: "apprise-provider",
  type: "apprise",
  name: "Saved Apprise",
  config: { urls: ["json://saved"] },
};
function wrapperWith(providers = [local, apprise]) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          notificationProviders: {
            items: providers,
            events: [TURN, ASK, UPDATE],
            appriseAvailable: true,
            loaded: true,
            loading: false,
          },
        }}
      >
        <SettingsSaveProvider>{children}</SettingsSaveProvider>
      </StateProvider>
    );
  };
}
function useHarness() {
  const state = useNotificationsState();
  return {
    state,
    actions: useNotificationsActions(state, async () => {}),
    save: useSaveRequest(state),
    dirty: useIsDirty(state),
    store: useAppStoreApi(),
  };
}
function transport() {
  const requests: Array<{
    path: string;
    method: string;
    body: unknown;
    respond: (data: unknown, status?: number) => void;
  }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(
      (path: string, init?: RequestInit) =>
        new Promise<Response>((resolve) => {
          requests.push({
            path,
            method: init?.method ?? "GET",
            body: init?.body ? JSON.parse(String(init.body)) : undefined,
            respond: (data, status = 200) =>
              resolve(
                new Response(JSON.stringify(data), {
                  status,
                  headers: { "Content-Type": "application/json" },
                }),
              ),
          });
        }),
    ),
  );
  return requests;
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// @covers AC-PLATFORM-NOTIFICATIONS-001.12
it.each(["add", "undo", "unchanged"])(
  "accepts canonical baseline without losing later event intent: %s",
  async (kind) => {
    const requests = transport();
    const view = renderHook(useHarness, { wrapper: wrapperWith([local]) });
    act(() =>
      view.result.current.actions.handleToggleEvent(view.result.current.state.providers[0], TURN),
    );
    let pending!: Promise<unknown>;
    act(() => {
      pending = view.result.current.save.run();
    });
    expect(requests[0].body).toEqual({ events: [TURN] });
    if (kind !== "unchanged")
      act(() =>
        view.result.current.actions.handleToggleEvent(
          view.result.current.state.providers[0],
          kind === "add" ? ASK : TURN,
        ),
      );
    const canonical = { ...local, events: [TURN, UPDATE], updated_at: "canonical" };
    await act(async () => {
      requests[0].respond(canonical);
      await pending;
    });
    const expected = { add: [TURN, UPDATE, ASK], undo: [UPDATE], unchanged: [TURN, UPDATE] }[kind]!;
    expect(view.result.current.store.getState().notificationProviders.items).toEqual([canonical]);
    expect(view.result.current.state.baselineProviders).toEqual([canonical]);
    expect(view.result.current.state.providers[0].events).toEqual(expected);
    expect(view.result.current.state.providers[0].updated_at).toBe("canonical");
    expect(view.result.current.dirty).toBe(kind !== "unchanged");
    if (kind === "unchanged") return;
    act(() => {
      pending = view.result.current.save.run();
    });
    expect(requests[1].body).toEqual({ events: expected });
    await act(async () => {
      requests[1].respond({ ...canonical, events: expected });
      await pending;
    });
    expect(view.result.current.dirty).toBe(false);
  },
);

it("retains later enabled/name/URL edits and raw buffers, then Reset uses canonical acknowledgement", async () => {
  const requests = transport();
  const view = renderHook(useHarness, { wrapper: wrapperWith([apprise]) });
  act(() => {
    view.result.current.actions.handleAppriseNameEdit(apprise.id, SUBMITTED_RAW_NAME);
    view.result.current.actions.handleAppriseEdit(apprise.id, ` ${SUBMITTED_URL} \n`);
  });
  let pending!: Promise<unknown>;
  act(() => {
    pending = view.result.current.save.run();
  });
  act(() => {
    view.result.current.actions.handleAppriseNameEdit(apprise.id, LATER_NAME);
    view.result.current.actions.handleAppriseEdit(apprise.id, ` ${LATER_URL} \n\n`);
    view.result.current.actions.updateProviderState(apprise.id, (p) => ({ ...p, enabled: false }));
  });
  const canonical = {
    ...apprise,
    name: SUBMITTED_NAME,
    config: { urls: [SUBMITTED_URL], server: true },
    updated_at: "canonical",
  };
  await act(async () => {
    requests[0].respond(canonical);
    await pending;
  });
  expect(view.result.current.state.providers[0]).toMatchObject({
    name: LATER_NAME,
    enabled: false,
    config: { urls: [LATER_URL], server: true },
    updated_at: "canonical",
  });
  expect(view.result.current.state.appriseNameEdits[apprise.id]).toBe(LATER_NAME);
  expect(view.result.current.state.appriseEdits[apprise.id]).toBe(` ${LATER_URL} \n\n`);
  expect(view.result.current.dirty).toBe(true);
  act(() => {
    pending = view.result.current.save.run();
  });
  expect(requests[1].body).toEqual({
    name: LATER_NAME,
    enabled: false,
    config: { urls: [LATER_URL], server: true },
  });
  const later = {
    ...canonical,
    name: LATER_NAME,
    enabled: false,
    config: { urls: [LATER_URL], server: true },
  };
  await act(async () => {
    requests[1].respond(later);
    await pending;
  });
  expect(view.result.current.dirty).toBe(false);
  act(() => view.result.current.actions.handleAppriseNameEdit(apprise.id, "Unsaved"));
  act(() => view.result.current.actions.discard());
  expect(view.result.current.state.providers).toEqual([later]);
  expect(view.result.current.state.appriseEdits[apprise.id]).toBe(LATER_URL);
  expect(view.result.current.dirty).toBe(false);
});

it("normalizes unchanged Apprise submissions while preserving a later independent event", async () => {
  const requests = transport();
  const view = renderHook(useHarness, { wrapper: wrapperWith([apprise]) });
  act(() => {
    view.result.current.actions.handleAppriseNameEdit(apprise.id, SUBMITTED_RAW_NAME);
    view.result.current.actions.handleAppriseEdit(apprise.id, ` ${SUBMITTED_URL} \n`);
  });
  let pending!: Promise<unknown>;
  act(() => {
    pending = view.result.current.save.run();
  });
  act(() =>
    view.result.current.actions.handleToggleEvent(view.result.current.state.providers[0], ASK),
  );
  const canonical = { ...apprise, name: SUBMITTED_NAME, config: { urls: [SUBMITTED_URL] } };
  await act(async () => {
    requests[0].respond(canonical);
    await pending;
  });
  expect(view.result.current.state.providers[0]).toEqual({ ...canonical, events: [ASK] });
  expect(view.result.current.state.appriseNameEdits[apprise.id]).toBe(SUBMITTED_NAME);
  expect(view.result.current.state.appriseEdits[apprise.id]).toBe(SUBMITTED_URL);
  act(() => view.result.current.actions.discard());
  expect(view.result.current.state.providers).toEqual([canonical]);
  expect(view.result.current.dirty).toBe(false);
});

it("keeps a later deletion out of the draft and persists it on the second Save", async () => {
  const requests = transport();
  const view = renderHook(useHarness, { wrapper: wrapperWith() });
  act(() => view.result.current.actions.handleToggleEvent(local, TURN));
  let pending!: Promise<unknown>;
  act(() => {
    pending = view.result.current.save.run();
  });
  act(() => view.result.current.actions.handleDeleteProvider(apprise.id));
  await act(async () => {
    requests[0].respond({ ...local, events: [TURN] });
    await pending;
  });
  expect(view.result.current.state.providers.map((p) => p.id)).toEqual([local.id]);
  expect(view.result.current.state.pendingDeletes).toEqual(new Set([apprise.id]));
  expect(view.result.current.state.baselineProviders.map((p) => p.id)).toEqual([
    local.id,
    apprise.id,
  ]);
  act(() => {
    pending = view.result.current.save.run();
  });
  expect(requests[1]).toMatchObject({
    path: expect.stringContaining(apprise.id),
    method: "DELETE",
  });
  await act(async () => {
    requests[1].respond({});
    await pending;
  });
  expect(view.result.current.state.pendingDeletes.size).toBe(0);
  expect(view.result.current.state.baselineProviders.map((p) => p.id)).toEqual([local.id]);
  expect(view.result.current.dirty).toBe(false);
});

it.each(["edited", "replacement", "closed", "unchanged"])(
  "acknowledges creation while respecting the current form: %s",
  async (kind) => {
    const requests = transport();
    const view = renderHook(useHarness, { wrapper: wrapperWith([local]) });
    act(() => view.result.current.actions.openAppriseForm("create"));
    act(() => {
      view.result.current.state.setAppriseName("First");
      view.result.current.state.setAppriseUrls(FIRST_URL);
    });
    let pending!: Promise<unknown>;
    await act(async () => {
      pending = view.result.current.save.run();
    });
    expect(requests[0]).toMatchObject({
      method: "POST",
      body: { name: "First", config: { urls: [FIRST_URL] } },
    });
    if (kind === "edited")
      act(() => {
        view.result.current.state.setAppriseName("Second");
        view.result.current.state.setAppriseUrls(SECOND_URL);
      });
    if (kind === "replacement" || kind === "closed")
      act(() => view.result.current.actions.cancelAppriseForm());
    if (kind === "replacement") {
      act(() => view.result.current.actions.openAppriseForm("create"));
      act(() => {
        view.result.current.state.setAppriseName("First");
        view.result.current.state.setAppriseUrls(FIRST_URL);
      });
    }
    const created = {
      ...apprise,
      id: "created",
      name: "First",
      config: { urls: [FIRST_URL] },
    };
    await act(async () => {
      requests[0].respond(created);
      await pending;
    });
    expect(view.result.current.state.baselineProviders).toEqual([local, created]);
    expect(view.result.current.state.providers).toEqual([local, created]);
    const retained = kind === "edited" || kind === "replacement";
    expect(view.result.current.state.showAppriseForm).toBe(retained);
    const name = kind === "edited" ? "Second" : "First";
    const urls = kind === "edited" ? SECOND_URL : FIRST_URL;
    expect(view.result.current.state.appriseName).toBe(retained ? name : "");
    expect(view.result.current.state.appriseUrls).toBe(retained ? urls : "");
    expect(view.result.current.dirty).toBe(retained);
    let acknowledged = [local, created];
    if (kind === "edited") {
      await act(async () => {
        pending = view.result.current.save.run();
      });
      expect(requests[1]).toMatchObject({
        method: "POST",
        body: { name: "Second", config: { urls: [SECOND_URL] } },
      });
      const second = {
        ...created,
        id: "created-second",
        name: "Second",
        config: { urls: [SECOND_URL] },
      };
      await act(async () => {
        requests[1].respond(second);
        await pending;
      });
      acknowledged = [...acknowledged, second];
      expect(view.result.current.dirty).toBe(false);
      expect(view.result.current.state.showAppriseForm).toBe(false);
    }
    act(() => view.result.current.actions.discard());
    expect(view.result.current.state.providers).toEqual(acknowledged);
    expect(view.result.current.dirty).toBe(false);
  },
);

it("preserves later edits and existing error state after a confirmed HTTP failure", async () => {
  const requests = transport();
  const view = renderHook(useHarness, { wrapper: wrapperWith([local]) });
  act(() => view.result.current.actions.handleToggleEvent(local, TURN));
  let pending!: Promise<unknown>;
  act(() => {
    pending = view.result.current.save.run().catch((error) => error);
  });
  act(() =>
    view.result.current.actions.handleToggleEvent(view.result.current.state.providers[0], ASK),
  );
  await act(async () => {
    requests[0].respond({ error: "Invalid selection" }, 400);
    await pending;
  });
  expect(view.result.current.save.status).toBe("error");
  expect(view.result.current.save.error?.message).toContain("Invalid selection");
  expect(view.result.current.state.providers[0].events).toEqual([TURN, ASK]);
  expect(view.result.current.state.baselineProviders).toEqual([local]);
  expect(view.result.current.store.getState().notificationProviders.items).toEqual([local]);
  expect(view.result.current.dirty).toBe(true);
});

it("keeps the real page/coordinator dirty and its event controls current until the next Save", async () => {
  const requests = transport();
  render(<NotificationsSettings />, { wrapper: wrapperWith([local]) });
  const table = within(screen.getByTestId("notification-events-desktop-table"));
  const turn = table.getByRole("checkbox", { name: "Agent turn finished for Local" });
  const ask = table.getByRole("checkbox", { name: "Agent needs an answer for Local" });
  fireEvent.click(turn);
  fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
  expect(requests).toHaveLength(1);
  fireEvent.click(ask);
  await act(async () => requests[0].respond({ ...local, events: [TURN] }));
  expect(ask.getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
  await waitFor(() =>
    expect((screen.getByRole("button", { name: SAVE_LABEL }) as HTMLButtonElement).disabled).toBe(
      false,
    ),
  );
  expect(turn.getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
  expect(ask.getAttribute("data-settings-dirty")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
  expect(requests).toHaveLength(2);
  expect(requests[1].body).toEqual({ events: [TURN, ASK] });
  await act(async () => requests[1].respond({ ...local, events: [TURN, ASK] }));
  await waitFor(() => expect(screen.queryByRole("button", { name: SAVE_LABEL })).toBeNull());
});

it("keeps later real Apprise card input values and Reset restores the acknowledged fields", async () => {
  const requests = transport();
  render(<NotificationsSettings />, { wrapper: wrapperWith([apprise]) });
  fireEvent.click(screen.getByRole("button", { name: "Edit" }));
  const nameInput = screen.getByDisplayValue("Saved Apprise") as HTMLInputElement;
  const urlsInput = screen.getByDisplayValue("json://saved") as HTMLTextAreaElement;
  fireEvent.change(nameInput, { target: { value: SUBMITTED_RAW_NAME } });
  fireEvent.change(urlsInput, { target: { value: ` ${SUBMITTED_URL} ` } });
  fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
  fireEvent.change(nameInput, { target: { value: LATER_NAME } });
  fireEvent.change(urlsInput, { target: { value: ` ${LATER_URL} ` } });
  await act(async () =>
    requests[0].respond({ ...apprise, name: SUBMITTED_NAME, config: { urls: [SUBMITTED_URL] } }),
  );
  expect(nameInput.value).toBe(LATER_NAME);
  expect(urlsInput.value).toBe(` ${LATER_URL} `);
  expect(
    screen.getByTestId("settings-floating-save").getAttribute("data-dirty-contributors"),
  ).not.toBe("");
  fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  await waitFor(() => expect(screen.queryByDisplayValue(LATER_NAME)).toBeNull());
  expect(screen.getAllByText(SUBMITTED_NAME).length).toBeGreaterThan(0);
  expect(requests).toHaveLength(1);
});

it("keeps real coordinator failure feedback and later draft until Reset", async () => {
  const requests = transport();
  render(<NotificationsSettings />, { wrapper: wrapperWith([local]) });
  const table = within(screen.getByTestId("notification-events-desktop-table"));
  const turn = table.getByRole("checkbox", { name: "Agent turn finished for Local" });
  const ask = table.getByRole("checkbox", { name: "Agent needs an answer for Local" });
  fireEvent.click(turn);
  fireEvent.click(screen.getByRole("button", { name: SAVE_LABEL }));
  fireEvent.click(ask);
  await act(async () => requests[0].respond({ error: "Invalid selection" }, 400));
  expect(screen.getByTestId("settings-floating-save").getAttribute("data-status")).toBe("error");
  expect(ask.getAttribute(CHECKED_ATTRIBUTE)).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  await waitFor(() => expect(turn.getAttribute(CHECKED_ATTRIBUTE)).toBe("false"));
  expect(ask.getAttribute(CHECKED_ATTRIBUTE)).toBe("false");
  expect(requests).toHaveLength(1);
});

it("removes only acknowledged deletion IDs and Reset restores surviving acknowledged providers", async () => {
  const requests = transport();
  const other = { ...apprise, id: "other-provider" };
  const view = renderHook(useHarness, { wrapper: wrapperWith([local, apprise, other]) });
  act(() => view.result.current.actions.handleDeleteProvider(apprise.id));
  let pending!: Promise<unknown>;
  act(() => {
    pending = view.result.current.save.run();
  });
  expect(requests[0]).toMatchObject({
    method: "DELETE",
    path: expect.stringContaining(apprise.id),
  });
  act(() => view.result.current.actions.handleDeleteProvider(other.id));
  await act(async () => {
    requests[0].respond({});
    await pending;
  });
  expect(view.result.current.state.providers).toEqual([local]);
  expect(view.result.current.state.pendingDeletes).toEqual(new Set([other.id]));
  expect(view.result.current.state.baselineProviders).toEqual([local, other]);
  expect(view.result.current.store.getState().notificationProviders.items).toEqual([local, other]);
  act(() => view.result.current.actions.discard());
  expect(view.result.current.state.providers).toEqual([local, other]);
  expect(view.result.current.state.pendingDeletes.size).toBe(0);
  expect(view.result.current.dirty).toBe(false);
});

it("normalizes unchanged open edit fields while keeping a later raw URL buffer", async () => {
  const requests = transport();
  const view = renderHook(useHarness, { wrapper: wrapperWith([apprise]) });
  act(() => view.result.current.actions.openAppriseForm("edit", apprise));
  act(() => {
    view.result.current.state.setAppriseName(SUBMITTED_RAW_NAME);
    view.result.current.actions.handleAppriseNameEdit(apprise.id, SUBMITTED_RAW_NAME);
    view.result.current.state.setAppriseUrls(` ${SUBMITTED_URL} `);
    view.result.current.actions.handleAppriseEdit(apprise.id, ` ${SUBMITTED_URL} `);
  });
  let pending!: Promise<unknown>;
  act(() => {
    pending = view.result.current.save.run();
  });
  act(() => {
    view.result.current.state.setAppriseUrls(` ${LATER_URL} \n`);
    view.result.current.actions.handleAppriseEdit(apprise.id, ` ${LATER_URL} \n`);
  });
  const canonical = { ...apprise, name: SUBMITTED_NAME, config: { urls: [SUBMITTED_URL] } };
  await act(async () => {
    requests[0].respond(canonical);
    await pending;
  });
  expect(view.result.current.state.appriseName).toBe(SUBMITTED_NAME);
  expect(view.result.current.state.appriseNameEdits[apprise.id]).toBe(SUBMITTED_NAME);
  expect(view.result.current.state.appriseUrls).toBe(` ${LATER_URL} \n`);
  expect(view.result.current.state.appriseEdits[apprise.id]).toBe(` ${LATER_URL} \n`);
  expect(view.result.current.state.baselineProviders).toEqual([canonical]);
  expect(view.result.current.dirty).toBe(true);
});
