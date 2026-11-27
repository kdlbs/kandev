import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { createElement, Fragment, type ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { activateLocale } from "@/lib/i18n";
import { useBackups } from "@/hooks/domains/system/use-backups";
import { FactoryResetDialog } from "./factory-reset-dialog";
import { RestoreDialog } from "./restore-dialog";

const restoreDialogTestState = vi.hoisted(() => ({
  job: null as { id?: string; state: string; message?: string } | null,
  restart: {
    phase: "idle" as const,
    errorMessage: null as string | null,
    isRestarting: false,
    start: vi.fn(),
    dismiss: vi.fn(),
  },
}));
const systemApiMocks = vi.hoisted(() => ({
  resetDatabase: vi.fn(),
  fetchBackups: vi.fn(),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  resetDatabase: systemApiMocks.resetDatabase,
  fetchBackups: systemApiMocks.fetchBackups,
}));

vi.mock("@/hooks/domains/system/use-system-jobs", () => ({
  useSystemJob: () => restoreDialogTestState.job,
  useSystemJobs: () => [],
}));

vi.mock("@/hooks/domains/system/use-kandev-restart", () => ({
  useKandevRestart: () => restoreDialogTestState.restart,
}));

const AUTH = {
  mode: "enabled" as const,
  authenticated: true,
  user: {
    id: "user-1",
    email: "user@example.com",
    display_name: "User",
    role: "admin" as const,
    status: "active" as const,
  },
};
const FACTORY_RESET_INPUT_TEST_ID = "system-factory-reset-input";
const FACTORY_RESET_CONFIRM_TEST_ID = "system-factory-reset-confirm";
const FACTORY_RESET_PENDING_TEST_ID = "system-factory-reset-pending";
const FACTORY_RESET_ERROR_TEST_ID = "system-factory-reset-error";
const FACTORY_RESET_TOKEN = "RESET";
let currentQueryClient: QueryClient | undefined;
let currentStore: StoreApi<AppState> | undefined;
let observeBackups = false;

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function StoreCapture() {
  currentStore = useAppStoreApi();
  return null;
}

function QueryCapture() {
  currentQueryClient = useQueryClient();
  return null;
}

function BackupObserver() {
  useBackups();
  return null;
}

function FactoryResetHarness({ children }: { children: ReactNode }) {
  return createElement(StateProvider, {
    initialState: { auth: AUTH },
    children: createElement(
      Fragment,
      null,
      createElement(StoreCapture),
      createElement(SystemInfoQueryProvider, {
        bootId: "reset-test-boot",
        children: createElement(
          Fragment,
          null,
          createElement(QueryCapture),
          observeBackups ? createElement(BackupObserver) : null,
          children,
        ),
      }),
    ),
  });
}

function renderFactoryReset(open = true) {
  return render(
    createElement(
      FactoryResetHarness,
      null,
      createElement(FactoryResetDialog, { open, onOpenChange: vi.fn() }),
    ),
  );
}

afterEach(() => {
  cleanup();
  currentQueryClient?.clear();
  currentQueryClient = undefined;
  currentStore = undefined;
  observeBackups = false;
  restoreDialogTestState.job = null;
  restoreDialogTestState.restart.start.mockReset();
  restoreDialogTestState.restart.dismiss.mockReset();
  systemApiMocks.resetDatabase.mockReset();
  systemApiMocks.fetchBackups.mockReset();
  systemApiMocks.fetchBackups.mockResolvedValue([]);
});

/**
 * Both dialogs gate their confirm button on `typed === CONFIRM_TOKEN` and send
 * the same token to the API, and both actions are irreversible. Translating a
 * token would leave a dialog the user cannot satisfy in that locale, and
 * nothing would fail until a second language shipped.
 */
describe("system type-to-confirm dialogs", () => {
  it("keeps the factory-reset token verbatim and gates the confirm button on it", () => {
    renderFactoryReset();

    // The whole reconstructed <Trans> sentence: a tag index drifting off its
    // <code> child reassembles the copy into fragments without failing
    // anything else in the suite.
    expect(screen.getByText(/to enable the confirm button/).textContent).toBe(
      "Type RESET to enable the confirm button. After the wipe completes you'll be asked to " +
        "quit and relaunch Kandev - the backend does not auto-restart.",
    );

    const input = screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID);
    const confirm = screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID) as HTMLButtonElement;
    expect(input.getAttribute("placeholder")).toBe("Type RESET to confirm");
    expect(confirm.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "reset" } });
    expect(confirm.disabled).toBe(true);

    fireEvent.change(input, { target: { value: FACTORY_RESET_TOKEN } });
    expect(confirm.disabled).toBe(false);
  });

  it("keeps the restore token verbatim and interpolates the snapshot name", () => {
    render(<RestoreDialog open onOpenChange={vi.fn()} name="kandev-20260803.db" />);

    // The filename is a value; only the frame around it is copy.
    expect(screen.getByText(/over the current database/).textContent).toBe(
      "Restore kandev-20260803.db over the current database. After the staged copy is in place " +
        "you will be asked to quit and relaunch Kandev so the new data is loaded fresh - the " +
        "backend does not auto-restart.",
    );

    const input = screen.getByTestId("system-restore-input");
    const confirm = screen.getByTestId("system-restore-confirm") as HTMLButtonElement;
    expect(input.getAttribute("placeholder")).toBe("Type RESTORE to confirm");
    expect(confirm.disabled).toBe(true);

    fireEvent.change(input, { target: { value: "RESTORE" } });
    expect(confirm.disabled).toBe(false);
  });

  it("requires a restart after a successful restore", () => {
    restoreDialogTestState.job = { state: "succeeded" };
    render(<RestoreDialog open onOpenChange={vi.fn()} name="snapshot-1.db" />);

    expect(screen.getByTestId("system-restore-restart")).toBeTruthy();
    expect(screen.queryByTestId("system-restore-close")).toBeNull();

    fireEvent.click(screen.getByTestId("system-restore-restart"));
    expect(restoreDialogTestState.restart.start).toHaveBeenCalledOnce();
  });
});

describe("factory reset backup-list refresh", () => {
  it.each(["succeeded", "failed"] as const)(
    "refreshes the backup list once when reset reaches %s, even after a possible snapshot publication",
    async (state) => {
      observeBackups = true;
      systemApiMocks.fetchBackups.mockResolvedValue([]);
      systemApiMocks.resetDatabase.mockResolvedValue({ job_id: "reset-job" });
      const view = renderFactoryReset();
      const backupQuery = () =>
        currentQueryClient
          ?.getQueryCache()
          .getAll()
          .find((query) => query.queryKey[0] === "system" && query.queryKey[1] === "backups");
      await waitFor(() => expect(backupQuery()?.state.data).toEqual([]));
      systemApiMocks.fetchBackups.mockClear();

      fireEvent.change(screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID), {
        target: { value: FACTORY_RESET_TOKEN },
      });
      fireEvent.click(screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID));
      await waitFor(() => expect(systemApiMocks.resetDatabase).toHaveBeenCalledOnce());
      await waitFor(() => expect(screen.getByTestId(FACTORY_RESET_PENDING_TEST_ID)).toBeTruthy());

      restoreDialogTestState.job = { id: "reset-job", state };
      view.rerender(
        createElement(
          FactoryResetHarness,
          null,
          createElement(FactoryResetDialog, { open: true, onOpenChange: vi.fn() }),
        ),
      );
      await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce());

      restoreDialogTestState.job = { id: "reset-job", state };
      view.rerender(
        createElement(
          FactoryResetHarness,
          null,
          createElement(FactoryResetDialog, { open: true, onOpenChange: vi.fn() }),
        ),
      );
      expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce();
      if (state === "succeeded")
        expect(screen.getByTestId("system-factory-reset-close")).toBeTruthy();
      else expect(screen.getByTestId(FACTORY_RESET_ERROR_TEST_ID)).toBeTruthy();
    },
  );

  it("clears pending and allows retry after reset acceptance fails", async () => {
    observeBackups = true;
    systemApiMocks.fetchBackups.mockResolvedValue([]);
    systemApiMocks.resetDatabase
      .mockRejectedValueOnce(new Error("request failed"))
      .mockResolvedValueOnce({ job_id: "retry-reset-job" });
    renderFactoryReset();
    await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce());
    systemApiMocks.fetchBackups.mockClear();
    const input = screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID) as HTMLInputElement;
    const confirm = screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID) as HTMLButtonElement;
    const cancel = screen.getByTestId("system-factory-reset-cancel") as HTMLButtonElement;
    fireEvent.change(input, {
      target: { value: FACTORY_RESET_TOKEN },
    });
    fireEvent.click(confirm);
    await waitFor(() =>
      expect(screen.getByTestId(FACTORY_RESET_ERROR_TEST_ID).textContent).toBe("request failed"),
    );
    expect(screen.queryByTestId(FACTORY_RESET_PENDING_TEST_ID)).toBeNull();
    expect(input.disabled).toBe(false);
    expect(confirm.disabled).toBe(false);
    expect(cancel.disabled).toBe(false);

    fireEvent.click(confirm);
    await waitFor(() => expect(systemApiMocks.resetDatabase).toHaveBeenCalledTimes(2));
    expect(systemApiMocks.resetDatabase).toHaveBeenNthCalledWith(2, FACTORY_RESET_TOKEN);
    expect(systemApiMocks.fetchBackups).not.toHaveBeenCalled();
  });
});

describe("factory reset rejection fencing", () => {
  it("keeps a newer reset pending when an older identity's acceptance rejects", async () => {
    observeBackups = true;
    systemApiMocks.fetchBackups.mockResolvedValue([]);
    let rejectOldAcceptance!: (reason?: unknown) => void;
    const oldAcceptance = new Promise<{ job_id: string }>((_, reject) => {
      rejectOldAcceptance = reject;
    });
    const newAcceptance = deferred<{ job_id: string }>();
    systemApiMocks.resetDatabase
      .mockReturnValueOnce(oldAcceptance)
      .mockReturnValueOnce(newAcceptance.promise);
    const view = renderFactoryReset();
    await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce());

    fireEvent.change(screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID), {
      target: { value: FACTORY_RESET_TOKEN },
    });
    fireEvent.click(screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID));
    await waitFor(() => expect(systemApiMocks.resetDatabase).toHaveBeenCalledOnce());

    act(() =>
      currentStore?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
    );
    await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledTimes(2));

    fireEvent.change(screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID), {
      target: { value: FACTORY_RESET_TOKEN },
    });
    fireEvent.click(screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID));
    await waitFor(() => expect(systemApiMocks.resetDatabase).toHaveBeenCalledTimes(2));
    expect(screen.getByTestId(FACTORY_RESET_PENDING_TEST_ID)).toBeTruthy();

    await act(async () => {
      rejectOldAcceptance(new Error("obsolete reset request failed"));
    });

    expect(screen.getByTestId(FACTORY_RESET_PENDING_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(FACTORY_RESET_ERROR_TEST_ID)).toBeNull();
    expect((screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID) as HTMLInputElement).disabled).toBe(
      true,
    );
    expect((screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID) as HTMLButtonElement).disabled).toBe(
      true,
    );
    expect((screen.getByTestId("system-factory-reset-cancel") as HTMLButtonElement).disabled).toBe(
      true,
    );

    await act(async () => {
      newAcceptance.resolve({ job_id: "new-reset-job" });
    });
    restoreDialogTestState.job = { id: "new-reset-job", state: "succeeded" };
    view.rerender(
      createElement(
        FactoryResetHarness,
        null,
        createElement(FactoryResetDialog, { open: true, onOpenChange: vi.fn() }),
      ),
    );
    await waitFor(() => expect(screen.getByTestId("system-factory-reset-close")).toBeTruthy());
  });
});

describe("factory reset identity fencing", () => {
  it("ignores a deferred reset acceptance after auth identity changes", async () => {
    observeBackups = true;
    systemApiMocks.fetchBackups.mockResolvedValue([]);
    const acceptance = deferred<{ job_id: string }>();
    systemApiMocks.resetDatabase.mockReturnValueOnce(acceptance.promise);
    renderFactoryReset();
    await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce());

    fireEvent.change(screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID), {
      target: { value: FACTORY_RESET_TOKEN },
    });
    fireEvent.click(screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID));
    await waitFor(() => expect(systemApiMocks.resetDatabase).toHaveBeenCalledOnce());

    act(() =>
      currentStore?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
    );
    await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledTimes(2));
    await act(async () => {
      acceptance.resolve({ job_id: "obsolete-reset-job" });
    });

    expect(systemApiMocks.fetchBackups).toHaveBeenCalledTimes(2);
    expect(screen.queryByTestId(FACTORY_RESET_PENDING_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(FACTORY_RESET_ERROR_TEST_ID)).toBeNull();
  });

  it("does not let an old reset job invalidate the backup list after auth changes", async () => {
    observeBackups = true;
    systemApiMocks.fetchBackups.mockResolvedValue([]);
    systemApiMocks.resetDatabase.mockResolvedValue({ job_id: "old-reset-job" });
    const view = renderFactoryReset();
    await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce());
    systemApiMocks.fetchBackups.mockClear();
    fireEvent.change(screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID), {
      target: { value: FACTORY_RESET_TOKEN },
    });
    fireEvent.click(screen.getByTestId(FACTORY_RESET_CONFIRM_TEST_ID));
    await waitFor(() => expect(screen.getByTestId(FACTORY_RESET_PENDING_TEST_ID)).toBeTruthy());

    act(() =>
      currentStore?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
    );
    await waitFor(() => expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce());
    restoreDialogTestState.job = { id: "old-reset-job", state: "failed" };
    view.rerender(
      createElement(
        FactoryResetHarness,
        null,
        createElement(FactoryResetDialog, { open: true, onOpenChange: vi.fn() }),
      ),
    );
    expect(systemApiMocks.fetchBackups).toHaveBeenCalledOnce();
  });
});

/**
 * The pseudo-locale oracle, at unit scale.
 *
 * `i18next/no-literal-string` runs in `mode: "jsx-only"`, so copy these dialogs
 * build outside JSX is invisible to lint and a clean lint is not proof they are
 * done. Under `pseudo` every catalog message is accented, so any word-like
 * ASCII left in the dialog is a literal that never reached the catalog — except
 * the confirmation token, which must stay verbatim for `typed === CONFIRM_TOKEN`
 * to be satisfiable.
 *
 * Accessibility copy is scanned too, and that is the point: the live pseudo
 * oracle only sees what is painted, so an `aria-label` is exactly the string
 * that survives a migration untranslated because nothing on screen looks wrong.
 */
describe("system type-to-confirm dialogs under the pseudo-locale", () => {
  const ACCENTED = /[À-ɏ]/;
  const WORDLIKE = /[A-Za-z]{4,}/;
  /** Attributes that carry copy, mirroring the eslint guard's include list. */
  const COPY_ATTRIBUTES = ["aria-label", "aria-description", "title", "placeholder", "alt"];

  function unlocalizedText(): string[] {
    const dialog = document.querySelector("[role=dialog]");
    if (!dialog) throw new Error("dialog did not render");
    const leftovers = new Set<string>();
    const consider = (value: string | null | undefined) => {
      const text = (value ?? "").trim();
      if (text && WORDLIKE.test(text) && !ACCENTED.test(text)) leftovers.add(text);
    };

    const walker = document.createTreeWalker(dialog, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      consider(node.textContent);
      node = walker.nextNode();
    }
    for (const element of [dialog, ...dialog.querySelectorAll("*")]) {
      for (const attribute of COPY_ATTRIBUTES) consider(element.getAttribute(attribute));
    }
    return [...leftovers];
  }

  beforeAll(async () => {
    await activateLocale("pseudo");
  });
  afterAll(async () => {
    await activateLocale("en");
  });

  /**
   * `Close` is `@kandev/ui`'s own `DialogContent` dismiss button. Localizing it
   * needs a strings-provider seam in that package (see docs/i18n.md, "Shared
   * UI"), so it is expected here rather than filtered out — if the package ever
   * gains that seam, this assertion is what tells us to drop it.
   */
  const UI_PACKAGE_CLOSE = "Close";

  it("leaves only the RESET token unaccented in the factory-reset dialog", () => {
    renderFactoryReset();
    // `<data-dir>/backups/` is a path placeholder and stays a value.
    expect(unlocalizedText().sort()).toEqual([
      "<data-dir>/backups/",
      UI_PACKAGE_CLOSE,
      FACTORY_RESET_TOKEN,
    ]);
    // Still typeable, and still announced, under a non-English locale.
    expect(screen.getByTestId(FACTORY_RESET_INPUT_TEST_ID).getAttribute("aria-label")).toContain(
      FACTORY_RESET_TOKEN,
    );
  });

  it("leaves only the RESTORE token and the snapshot name unaccented", () => {
    render(<RestoreDialog open onOpenChange={vi.fn()} name="snapshot-1.db" />);
    expect(unlocalizedText().sort()).toEqual([UI_PACKAGE_CLOSE, "RESTORE", "snapshot-1.db"]);
    expect(screen.getByTestId("system-restore-input").getAttribute("aria-label")).toContain(
      "RESTORE",
    );
  });
});
