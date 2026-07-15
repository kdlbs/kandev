import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import Link from "@/components/routing/app-link";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import {
  SettingsSaveProvider,
  SettingsSaveCancelledError,
  useSettingsSaveContributor,
  type SettingsSaveRevision,
} from "./settings-save-provider";

type Deferred = {
  promise: Promise<void>;
  resolve: () => void;
};

const SAVE_CHANGES_LABEL = "Save changes";
const APPEARANCE_PATH = "/settings/general/appearance";
const TERMINAL_PATH = "/settings/general/terminal";

function deferred(): Deferred {
  let resolve: () => void = () => undefined;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function DraftContributor({
  id,
  order,
  initialRevision = 1,
  canSave = true,
  onSave,
  onDiscard,
}: {
  id: string;
  order?: number;
  initialRevision?: number;
  canSave?: boolean;
  onSave: (revision: SettingsSaveRevision) => Promise<void> | void;
  onDiscard?: () => Promise<void> | void;
}) {
  const [revision, setRevision] = useState(initialRevision);
  const [savedRevision, setSavedRevision] = useState(0);

  useSettingsSaveContributor({
    id,
    order,
    revision,
    isDirty: revision !== savedRevision,
    canSave,
    invalidReason: canSave ? undefined : `${id} is invalid`,
    save: async (submittedRevision) => {
      await onSave(submittedRevision);
      setSavedRevision(submittedRevision as number);
    },
    discard: async () => {
      await onDiscard?.();
      setRevision(savedRevision);
    },
  });

  return (
    <button type="button" onClick={() => setRevision((current) => current + 1)}>
      Edit {id}
    </button>
  );
}

function BooleanDraftContributor({ pending }: { pending: Deferred }) {
  const [draft, setDraft] = useState(true);
  const [baseline, setBaseline] = useState(false);

  useSettingsSaveContributor({
    id: "toggle",
    revision: Number(draft),
    isDirty: draft !== baseline,
    save: async (revision) => {
      await pending.promise;
      setBaseline(Boolean(revision));
    },
    discard: () => setDraft(baseline),
  });

  return (
    <button type="button" onClick={() => setDraft((current) => !current)}>
      Toggle
    </button>
  );
}

afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
  vi.restoreAllMocks();
});

describe("SettingsSaveProvider", () => {
  it("saves dirty contributors in stable order and retries only failures", async () => {
    const calls: string[] = [];
    let failSecond = true;

    render(
      <SettingsSaveProvider>
        <DraftContributor
          id="second"
          order={20}
          onSave={() => {
            calls.push("second");
          }}
        />
        <DraftContributor
          id="first"
          order={10}
          onSave={() => {
            calls.push("first");
          }}
        />
        <DraftContributor
          id="failing"
          order={30}
          onSave={() => {
            calls.push("failing");
            if (failSecond) {
              failSecond = false;
              throw new Error("network unavailable");
            }
          }}
        />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    expect(await screen.findByText("Couldn't save")).toBeTruthy();
    expect(calls).toEqual(["first", "second", "failing"]);

    fireEvent.click(screen.getByRole("button", { name: "Retry save" }));

    await waitFor(() => expect(screen.queryByRole("button", { name: "Retry save" })).toBeNull());
    expect(calls).toEqual(["first", "second", "failing", "failing"]);
  });

  it("keeps a newer revision dirty when an earlier save finishes", async () => {
    const pending = deferred();
    const revisions: SettingsSaveRevision[] = [];

    render(
      <SettingsSaveProvider>
        <DraftContributor
          id="profile"
          onSave={async (revision) => {
            revisions.push(revision);
            await pending.promise;
          }}
        />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
    expect(screen.getByRole("button", { name: "Saving changes" }).hasAttribute("disabled")).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "Edit profile" }));

    await act(async () => pending.resolve());

    expect(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL })).toBeTruthy();
    expect(revisions).toEqual([1]);
  });
});

describe("SettingsSaveProvider save state", () => {
  it("keeps a cancelled save pending without reporting an error", async () => {
    render(
      <SettingsSaveProvider>
        <DraftContributor
          id="ssh"
          onSave={() => {
            throw new SettingsSaveCancelledError();
          }}
        />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    expect(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL })).toBeTruthy();
    expect(screen.queryByText("Couldn't save")).toBeNull();
  });

  it("keeps a toggle dirty when it returns to the old baseline during an in-flight save", async () => {
    const pending = deferred();

    render(
      <SettingsSaveProvider>
        <BooleanDraftContributor pending={pending} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "Toggle" }));
    await act(async () => pending.resolve());

    expect(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL })).toBeTruthy();
  });

  it("disables saving while a dirty contributor is invalid", async () => {
    render(
      <SettingsSaveProvider>
        <DraftContributor id="invalid-profile" canSave={false} onSave={vi.fn()} />
      </SettingsSaveProvider>,
    );

    const save = await screen.findByRole("button", { name: SAVE_CHANGES_LABEL });
    expect(save.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("invalid-profile is invalid")).toBeTruthy();
  });

  it("briefly confirms a successful save", async () => {
    render(
      <SettingsSaveProvider>
        <DraftContributor id="appearance" onSave={vi.fn()} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: SAVE_CHANGES_LABEL }));

    expect(await screen.findByRole("button", { name: "Saved" })).toBeTruthy();
  });
});

describe("SettingsSaveProvider navigation", () => {
  it("offers discard before in-app navigation and warns before reload", async () => {
    window.history.replaceState({}, "", APPEARANCE_PATH);

    render(
      <SettingsSaveProvider>
        <DraftContributor id="appearance" onSave={vi.fn()} />
        <Link href={TERMINAL_PATH}>Terminal</Link>
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("link", { name: "Terminal" }));

    expect(await screen.findByRole("alertdialog")).toBeTruthy();
    expect(window.location.pathname).toBe(APPEARANCE_PATH);

    const unload = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(unload);
    expect(unload.defaultPrevented).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Discard and leave" }));
    await waitFor(() => expect(window.location.pathname).toBe(TERMINAL_PATH));
  });

  it("runs an asynchronous discard only once while leaving", async () => {
    window.history.replaceState({}, "", APPEARANCE_PATH);
    const pending = deferred();
    const onDiscard = vi.fn(() => pending.promise);

    render(
      <SettingsSaveProvider>
        <DraftContributor id="appearance" onSave={vi.fn()} onDiscard={onDiscard} />
        <Link href={TERMINAL_PATH}>Terminal</Link>
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("link", { name: "Terminal" }));
    const discard = await screen.findByRole("button", { name: "Discard and leave" });
    fireEvent.click(discard);
    fireEvent.click(discard);

    expect(onDiscard).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Discarding..." }).hasAttribute("disabled")).toBe(
      true,
    );

    await act(async () => pending.resolve());
    await waitFor(() => expect(window.location.pathname).toBe(TERMINAL_PATH));
  });

  it("saves before approved navigation and stays put when saving fails", async () => {
    window.history.replaceState({}, "", APPEARANCE_PATH);
    let shouldFail = true;

    render(
      <SettingsSaveProvider>
        <DraftContributor
          id="appearance"
          onSave={() => {
            if (shouldFail) throw new Error("save failed");
          }}
        />
        <Link href={TERMINAL_PATH}>Terminal</Link>
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("link", { name: "Terminal" }));
    fireEvent.click(await screen.findByRole("button", { name: "Save and leave" }));

    expect(await screen.findByText("Couldn't save")).toBeTruthy();
    expect(window.location.pathname).toBe(APPEARANCE_PATH);

    shouldFail = false;
    fireEvent.click(screen.getByRole("button", { name: "Save and leave" }));
    await waitFor(() => expect(window.location.pathname).toBe(TERMINAL_PATH));
  });

  it("hides the action after the last dirty contributor unregisters", async () => {
    function RemovableDraft() {
      const [visible, setVisible] = useState(true);
      return (
        <>
          {visible && <DraftContributor id="temporary" onSave={vi.fn()} />}
          <button type="button" onClick={() => setVisible(false)}>
            Remove draft
          </button>
        </>
      );
    }

    render(
      <SettingsSaveProvider>
        <RemovableDraft />
      </SettingsSaveProvider>,
    );

    expect(await screen.findByTestId("settings-floating-save")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove draft" }));
    await waitFor(() => expect(screen.queryByTestId("settings-floating-save")).toBeNull());
  });
});
