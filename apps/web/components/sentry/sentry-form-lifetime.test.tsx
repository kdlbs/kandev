import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import type { SentryConfig } from "@/lib/types/sentry";
import { SentryConnectionSection } from "./sentry-settings";

const ADD_TEST = "sentry-add-test-button";
const INSTANCE_CARD = "sentry-instance-card";
const instance: SentryConfig = {
  id: "instance-a",
  workspaceId: "workspace-a",
  name: "Original instance",
  authMethod: "auth_token",
  url: "https://sentry.fixture.test",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-10-21T00:00:00Z",
  updatedAt: "2026-10-21T00:00:00Z",
};
const other = { ...instance, id: "instance-b", name: "Other instance" };
const submitted = {
  name: "Submitted",
  url: "https://submitted.fixture.test",
  secret: "synthetic-submitted",
};
const newer = {
  name: "New unsaved instance",
  url: "https://new.fixture.test",
  secret: "synthetic-new",
};
let coordinator: ReturnType<typeof useSettingsSaveCoordinator>;
function Probe() {
  coordinator = useSettingsSaveCoordinator();
  return null;
}
function Harness({ visible = true }: { visible?: boolean }) {
  return (
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <SettingsSaveProvider>
            {visible && <SentryConnectionSection workspaceId="workspace-a" />}
            <Probe />
          </SettingsSaveProvider>
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.unstubAllGlobals();
});

function fixture() {
  const held = Promise.withResolvers<Response>();
  const writes: Array<{ method: string; path: string; body: Record<string, string> }> = [];
  let rows = [instance, other];
  let reads = 0;
  const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      const url = new URL(value, "http://localhost");
      expect(url.searchParams.get("workspace_id")).toBe("workspace-a");
      if (init?.method === "POST" || init?.method === "PUT") {
        const write = {
          method: init.method,
          path: url.pathname,
          body: JSON.parse(init.body as string),
        };
        writes.push(write);
        if (writes.length === 1) return held.promise;
        return Promise.resolve(json({ ...instance, ...write.body, id: "new-saved" }));
      }
      expect(url.pathname).toBe("/api/v1/sentry/instances");
      reads++;
      return Promise.resolve(json({ instances: rows }));
    }),
  );
  return {
    writes,
    setRows: (next: SentryConfig[]) => {
      rows = next;
    },
    get reads() {
      return reads;
    },
    settle: (success: boolean) => {
      rows = [...rows, { ...instance, ...submitted, id: "old-saved" }];
      held.resolve(
        json(
          success ? { ...instance, ...submitted } : { error: "old save rejected" },
          success ? 200 : 500,
        ),
      );
    },
  };
}

function open(kind: "add" | "edit", name = instance.name) {
  if (kind === "add") fireEvent.click(screen.getByTestId("sentry-add-instance-button"));
  else {
    const card = screen.getAllByTestId(INSTANCE_CARD).find((el) => el.textContent?.includes(name));
    expect(card).toBeTruthy();
    fireEvent.click(within(card!).getByTestId("sentry-instance-edit-button"));
  }
}
function fill(kind: "add" | "edit", draft: typeof submitted) {
  for (const [field, value] of Object.entries(draft)) {
    fireEvent.change(screen.getByTestId(`sentry-${kind}-${field}-input`), { target: { value } });
  }
}
function beginSave() {
  let pending!: ReturnType<typeof coordinator.saveAll>;
  act(() => {
    pending = coordinator.saveAll();
  });
  return pending;
}
async function settle(
  f: ReturnType<typeof fixture>,
  pending: ReturnType<typeof beginSave>,
  success: boolean,
) {
  let result!: Awaited<typeof pending>;
  await act(async () => {
    f.settle(success);
    result = await pending;
  });
  return result;
}

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1, AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.2
// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.4
describe.each(["add", "edit"] as const)("old %s", (oldKind) => {
  for (const replacement of ["add", "other-edit", "same-edit"] as const) {
    for (const success of [true, false]) {
      it(`${success ? "success" : "failure"} preserves ${replacement} and its next Save/Cancel`, async () => {
        const f = fixture();
        render(<Harness />);
        await screen.findAllByTestId(INSTANCE_CARD);
        open(oldKind);
        fill(oldKind, submitted);
        const pending = beginSave();
        await waitFor(() => expect(f.writes).toHaveLength(1));
        expect(f.writes[0]).toEqual({
          method: oldKind === "add" ? "POST" : "PUT",
          path: `/api/v1/sentry/instances${oldKind === "edit" ? "/instance-a" : ""}`,
          body: {
            ...submitted,
            authMethod: "auth_token",
            ...(oldKind === "add" ? { workspaceId: "workspace-a" } : {}),
          },
        });
        fireEvent.click(screen.getByTestId(`sentry-${oldKind}-cancel-button`));
        const nextKind = replacement === "add" ? "add" : "edit";
        open(nextKind, replacement === "other-edit" ? other.name : instance.name);
        fill(nextKind, newer);
        const input = screen.getByTestId(`sentry-${nextKind}-name-input`) as HTMLInputElement;
        input.focus();
        input.setSelectionRange(2, 7);
        const reads = f.reads;
        const result = await settle(f, pending, success);
        expect(result.canLeave).toBe(false);
        expect(screen.getByTestId(`sentry-${nextKind}-name-input`)).toBe(input);
        for (const [field, value] of Object.entries(newer)) {
          expect(screen.getByTestId(`sentry-${nextKind}-${field}-input`)).toHaveProperty(
            "value",
            value,
          );
        }
        expect(document.activeElement).toBe(input);
        expect([input.selectionStart, input.selectionEnd]).toEqual([2, 7]);
        expect(coordinator.hasDirty).toBe(true);
        expect(coordinator.contributorStates.filter((c) => c.isDirty)).toEqual([
          expect.objectContaining({ saveFailed: false, invalid: false }),
        ]);
        expect(
          screen.getByTestId(`sentry-${nextKind}-form`).getAttribute("data-settings-dirty"),
        ).toBe("true");
        expect(screen.getByTestId(`sentry-${nextKind}-test-button`)).toHaveProperty(
          "disabled",
          false,
        );
        expect(screen.getByTestId(`sentry-${nextKind}-cancel-button`)).toHaveProperty(
          "disabled",
          false,
        );
        expect(
          screen.getByText(success ? "Sentry instance saved" : /old save rejected/),
        ).toBeTruthy();
        if (success) expect(f.reads).toBeGreaterThan(reads);
        expect(f.writes).toHaveLength(1);
        expect(
          screen.getByRole("button", {
            name: success ? "Save changes" : "Retry save",
          }),
        ).toHaveProperty("disabled", false);
        await act(async () => {
          await coordinator.saveAll();
        });
        expect(f.writes[1].body).toEqual(expect.objectContaining(newer));
        expect(screen.queryByTestId(`sentry-${nextKind}-form`)).toBeNull();
        open("add");
        fireEvent.click(screen.getByTestId("sentry-add-cancel-button"));
        expect(coordinator.hasDirty).toBe(false);
        expect(f.writes).toHaveLength(2);
      });
    }
  }
});

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1
for (const kind of ["add", "edit"] as const) {
  it.each([true, false])(
    `isolates an identical reopened ${kind} on old success=%s`,
    async (success) => {
      const f = fixture();
      render(<Harness />);
      await screen.findAllByTestId(INSTANCE_CARD);
      open(kind);
      fill(kind, submitted);
      const pending = beginSave();
      await waitFor(() => expect(f.writes).toHaveLength(1));
      fireEvent.click(screen.getByTestId(`sentry-${kind}-cancel-button`));
      open(kind);
      fill(kind, submitted);
      const result = await settle(f, pending, success);
      expect(screen.getByTestId(`sentry-${kind}-name-input`)).toHaveProperty(
        "value",
        submitted.name,
      );
      expect(result.canLeave).toBe(false);
      expect(coordinator.contributorStates.filter((c) => c.isDirty)).toEqual([
        expect.objectContaining({ isDirty: true, saveFailed: false }),
      ]);
    },
  );
}

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.3
it.each(["add", "edit"] as const)(
  "current %s failure retains draft and retries successfully",
  async (kind) => {
    const f = fixture();
    render(<Harness />);
    await screen.findAllByTestId(INSTANCE_CARD);
    open(kind);
    fill(kind, submitted);
    const pending = beginSave();
    await waitFor(() => expect(f.writes).toHaveLength(1));
    await settle(f, pending, false);
    expect(screen.getByTestId(`sentry-${kind}-name-input`)).toHaveProperty("value", submitted.name);
    expect(coordinator.hasDirty).toBe(true);
    expect(coordinator.contributorStates.find((c) => c.isDirty)?.saveFailed).toBe(true);
    await act(async () => {
      await coordinator.saveAll();
    });
    expect(screen.queryByTestId(`sentry-${kind}-form`)).toBeNull();
    expect(coordinator.hasDirty).toBe(false);
  },
);

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.2
it("an accepted write survives section unmount without closing a remounted form", async () => {
  const f = fixture();
  const view = render(<Harness />);
  await screen.findAllByTestId(INSTANCE_CARD);
  open("add");
  fill("add", submitted);
  const pending = beginSave();
  await waitFor(() => expect(f.writes).toHaveLength(1));
  view.rerender(<Harness visible={false} />);
  view.rerender(<Harness />);
  await screen.findAllByTestId(INSTANCE_CARD);
  open("add");
  fill("add", newer);
  await settle(f, pending, true);
  expect(screen.getByTestId("sentry-add-name-input")).toHaveProperty("value", newer.name);
  expect(coordinator.hasDirty).toBe(true);
});

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1
it.each([true, false])(
  "old connection test success=%s cannot settle the replacement test",
  async (success) => {
    fixture();
    const fetchInstances = globalThis.fetch;
    const oldTest = Promise.withResolvers<Response>();
    const currentTest = Promise.withResolvers<Response>();
    let tests = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn((value: string, init?: RequestInit) => {
        if (new URL(value, "http://localhost").pathname.endsWith("/test-connection")) {
          tests++;
          return tests === 1 ? oldTest.promise : currentTest.promise;
        }
        return fetchInstances(value, init);
      }),
    );
    render(<Harness />);
    await screen.findAllByTestId(INSTANCE_CARD);
    open("add");
    fill("add", submitted);
    fireEvent.click(screen.getByTestId(ADD_TEST));
    await waitFor(() => expect(tests).toBe(1));
    fireEvent.click(screen.getByTestId("sentry-add-cancel-button"));
    open("add");
    fill("add", newer);
    fireEvent.click(screen.getByTestId(ADD_TEST));
    await waitFor(() => expect(tests).toBe(2));
    await act(async () => {
      oldTest.resolve(
        new Response(
          JSON.stringify(
            success
              ? { ok: true, displayName: "old-account" }
              : { ok: false, error: "old-test-error" },
          ),
        ),
      );
    });
    expect(screen.getByTestId(ADD_TEST)).toHaveProperty("disabled", true);
    expect(screen.queryByText(/old-account|old-test-error/)).toBeNull();
    await act(async () => {
      currentTest.resolve(
        new Response(JSON.stringify({ ok: true, displayName: "current-account" })),
      );
    });
    await screen.findByText(/current-account/);
    expect(screen.getByTestId(ADD_TEST)).toHaveProperty("disabled", false);
    expect(screen.getByTestId("sentry-add-name-input")).toHaveProperty("value", newer.name);
  },
);

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1
it("preserves a new form mount after polling removes and restores the same instance", async () => {
  vi.useFakeTimers();
  const f = fixture();
  render(<Harness />);
  await act(async () => {
    await Promise.resolve();
  });
  expect(screen.getAllByTestId(INSTANCE_CARD)).toHaveLength(2);
  open("edit");
  fill("edit", submitted);
  const pending = beginSave();
  expect(f.writes).toHaveLength(1);
  f.setRows([other]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(90_000);
  });
  expect(screen.queryByTestId("sentry-edit-form")).toBeNull();
  f.setRows([instance, other]);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(90_000);
  });
  fill("edit", newer);
  await settle(f, pending, true);
  expect(screen.getByTestId("sentry-edit-name-input")).toHaveProperty("value", newer.name);
  expect(coordinator.hasDirty).toBe(true);
});

// @covers AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1
it("makes the newer draft saveable while an older save's list refresh is pending", async () => {
  const f = fixture();
  const fetchWrites = globalThis.fetch;
  const refresh = Promise.withResolvers<Response>();
  let holdRefresh = false;
  let refreshStarted = false;
  vi.stubGlobal(
    "fetch",
    vi.fn((value: string, init?: RequestInit) => {
      if (holdRefresh && !init?.method) {
        refreshStarted = true;
        return refresh.promise;
      }
      return fetchWrites(value, init);
    }),
  );
  render(<Harness />);
  await screen.findAllByTestId(INSTANCE_CARD);
  open("add");
  fill("add", submitted);
  const pending = beginSave();
  await waitFor(() => expect(f.writes).toHaveLength(1));
  fireEvent.click(screen.getByTestId("sentry-add-cancel-button"));
  open("add");
  fill("add", newer);
  holdRefresh = true;
  try {
    await act(async () => {
      f.settle(true);
    });
    await waitFor(() => expect(refreshStarted).toBe(true));
    expect(screen.getByRole("button", { name: "Save changes" })).toHaveProperty("disabled", false);
    expect(screen.getByTestId("sentry-add-name-input")).toHaveProperty("value", newer.name);
  } finally {
    await act(async () => {
      refresh.resolve(new Response(JSON.stringify({ instances: [instance, other] })));
      await pending;
    });
  }
});
