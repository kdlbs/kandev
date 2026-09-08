import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useRef, useState } from "react";
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
const RESET_LABEL = "Reset";
const RETRY_RESET_LABEL = "Retry reset";
const COULDNT_SAVE_LABEL = "Couldn't save";
const COULDNT_RESET_LABEL = "Couldn't reset";
const APPEARANCE_ID = "appearance";
const FLOATING_SAVE_TEST_ID = "settings-floating-save";
const SAVE_AND_LEAVE_LABEL = "Save and leave";
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
  const latestRevisionRef = useRef(revision);
  latestRevisionRef.current = revision;

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
    discard: async (submittedRevision) => {
      await onDiscard?.();
      if (
        submittedRevision !== undefined &&
        !Object.is(submittedRevision, latestRevisionRef.current)
      ) {
        return;
      }
      setRevision(savedRevision);
    },
  });

  return (
    <button type="button" onClick={() => setRevision((current) => current + 1)}>
      Edit {id}
    </button>
  );
}

function InvalidWithoutReasonContributor() {
  useSettingsSaveContributor({
    id: "invalid-without-reason",
    revision: 1,
    isDirty: true,
    canSave: false,
    save: vi.fn(),
    discard: vi.fn(),
  });
  return null;
}

/** A contributor that starts clean (revision equals savedRevision) and dirties on Edit. */
function CleanStartDraftContributor({
  id,
  onSave,
}: {
  id: string;
  onSave: () => Promise<void> | void;
}) {
  const [revision, setRevision] = useState(1);
  const [savedRevision, setSavedRevision] = useState(1); // starts clean

  useSettingsSaveContributor({
    id,
    revision,
    isDirty: revision !== savedRevision,
    save: async () => {
      await onSave();
      setSavedRevision(revision);
    },
    discard: () => setRevision(savedRevision),
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

function ResettingDraftContributor() {
  const [mode, setMode] = useState<"create" | "idle">("create");

  useSettingsSaveContributor({
    id: "resetting-draft",
    revision: mode,
    isDirty: mode === "create",
    save: () => setMode("idle"),
    discard: () => setMode("idle"),
  });

  return null;
}

afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
  vi.restoreAllMocks();
});

describe("SettingsSaveProvider", () => {
  it("keeps the generic standalone save action above the app status bar", async () => {
    render(
      <SettingsSaveProvider>
        <DraftContributor id={APPEARANCE_ID} onSave={vi.fn()} />
      </SettingsSaveProvider>,
    );

    expect((await screen.findByTestId(FLOATING_SAVE_TEST_ID)).className).toContain(
      "var(--app-status-bar-height)",
    );
  });

  it("does not double-reserve status-bar space for the content-hosted save action", async () => {
    render(
      <SettingsSaveProvider placement="content">
        <DraftContributor id={APPEARANCE_ID} onSave={vi.fn()} />
      </SettingsSaveProvider>,
    );

    expect((await screen.findByTestId(FLOATING_SAVE_TEST_ID)).className).not.toContain(
      "var(--app-status-bar-height)",
    );
  });
});

describe("SettingsFloatingSave", () => {
  it("renders a centered neutral surface with Reset and Save changes", async () => {
    render(
      <SettingsSaveProvider>
        <DraftContributor id={APPEARANCE_ID} onSave={vi.fn()} />
      </SettingsSaveProvider>,
    );

    const surface = await screen.findByTestId(FLOATING_SAVE_TEST_ID);
    expect(surface.className).toContain("justify-center");
    expect(surface.className).toContain("inset-x-0");
    expect(surface.getAttribute("data-status")).toBe("dirty");
    expect(screen.getByRole("button", { name: RESET_LABEL })).toBeTruthy();
    expect(screen.getByRole("button", { name: SAVE_CHANGES_LABEL }).className).toContain(
      "bg-success",
    );
    expect(surface.className).not.toContain("bg-success");
  });

  it("resets every dirty contributor without saving and hides the surface", async () => {
    const discarded: string[] = [];
    const saved = vi.fn();

    render(
      <SettingsSaveProvider>
        <DraftContributor
          id="second"
          order={20}
          onSave={saved}
          onDiscard={() => {
            discarded.push("second");
          }}
        />
        <DraftContributor
          id="first"
          order={10}
          onSave={saved}
          onDiscard={() => {
            discarded.push("first");
          }}
        />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: RESET_LABEL }));

    await waitFor(() => expect(screen.queryByTestId(FLOATING_SAVE_TEST_ID)).toBeNull());
    expect(discarded).toEqual(["first", "second"]);
    expect(saved).not.toHaveBeenCalled();
  });

  it("prevents duplicate resets while a contributor is discarding", async () => {
    const pending = deferred();
    const discarded = vi.fn(() => pending.promise);

    render(
      <SettingsSaveProvider>
        <DraftContributor id={APPEARANCE_ID} onSave={vi.fn()} onDiscard={discarded} />
      </SettingsSaveProvider>,
    );

    const reset = await screen.findByRole("button", { name: RESET_LABEL });
    fireEvent.click(reset);
    fireEvent.click(reset);

    expect(discarded).toHaveBeenCalledOnce();
    expect((reset as HTMLButtonElement).disabled).toBe(true);

    await act(async () => pending.resolve());
    await waitFor(() => expect(screen.queryByTestId(FLOATING_SAVE_TEST_ID)).toBeNull());
  });

  it("keeps the route dirty when Reset fails", async () => {
    let shouldFail = true;
    const discarded = vi.fn(() => {
      if (shouldFail) {
        shouldFail = false;
        throw new Error("discard failed");
      }
    });

    render(
      <SettingsSaveProvider>
        <DraftContributor id={APPEARANCE_ID} onSave={vi.fn()} onDiscard={discarded} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: RESET_LABEL }));

    expect(await screen.findByText(COULDNT_RESET_LABEL)).toBeTruthy();
    expect(screen.queryByText(COULDNT_SAVE_LABEL)).toBeNull();
    expect(screen.getByTestId(FLOATING_SAVE_TEST_ID)).toBeTruthy();
    expect((screen.getByRole("button", { name: RESET_LABEL }) as HTMLButtonElement).disabled).toBe(
      false,
    );

    fireEvent.click(screen.getByRole("button", { name: RETRY_RESET_LABEL }));
    await waitFor(() => expect(screen.queryByTestId(FLOATING_SAVE_TEST_ID)).toBeNull());
    expect(discarded).toHaveBeenCalledTimes(2);
  });
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

    expect(await screen.findByText(COULDNT_SAVE_LABEL)).toBeTruthy();
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

  it("disables saving when an invalid contributor has no reason text", async () => {
    render(
      <SettingsSaveProvider>
        <InvalidWithoutReasonContributor />
      </SettingsSaveProvider>,
    );

    const save = await screen.findByRole("button", { name: SAVE_CHANGES_LABEL });
    expect(save.hasAttribute("disabled")).toBe(true);
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
    fireEvent.click(await screen.findByRole("button", { name: SAVE_AND_LEAVE_LABEL }));

    expect(await screen.findByText("Couldn't save")).toBeTruthy();
    expect(window.location.pathname).toBe(APPEARANCE_PATH);

    shouldFail = false;
    fireEvent.click(screen.getByRole("button", { name: SAVE_AND_LEAVE_LABEL }));
    await waitFor(() => expect(window.location.pathname).toBe(TERMINAL_PATH));
  });

  it("continues navigation when a successful save resets the draft revision", async () => {
    window.history.replaceState({}, "", APPEARANCE_PATH);

    render(
      <SettingsSaveProvider>
        <ResettingDraftContributor />
        <Link href={TERMINAL_PATH}>Terminal</Link>
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("link", { name: "Terminal" }));
    fireEvent.click(await screen.findByRole("button", { name: SAVE_AND_LEAVE_LABEL }));

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

    expect(await screen.findByTestId(FLOATING_SAVE_TEST_ID)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove draft" }));
    await waitFor(() => expect(screen.queryByTestId(FLOATING_SAVE_TEST_ID)).toBeNull());
  });
});

describe("SettingsSaveProvider mid-save edits", () => {
  it("stays put when a contributor becomes dirty while the save is in flight", async () => {
    window.history.replaceState({}, "", APPEARANCE_PATH);
    const pending = deferred();

    render(
      <SettingsSaveProvider>
        <DraftContributor id="appearance" onSave={() => pending.promise} />
        <CleanStartDraftContributor id="mcp" onSave={vi.fn()} />
        <Link href={TERMINAL_PATH}>Terminal</Link>
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Edit appearance" }));
    fireEvent.click(screen.getByRole("link", { name: "Terminal" }));
    fireEvent.click(await screen.findByRole("button", { name: SAVE_AND_LEAVE_LABEL }));
    // The MCP contributor (clean at the save snapshot) becomes dirty while
    // the appearance save is pending. Radix aria-hides the page behind the
    // modal, so query with hidden:true.
    fireEvent.click(screen.getByRole("button", { name: "Edit mcp", hidden: true }));

    await act(async () => pending.resolve());

    // The newly dirty contributor blocks leaving: the navigation must not
    // proceed (saveAll reports canLeave=false).
    await waitFor(() => expect(window.location.pathname).toBe(APPEARANCE_PATH));
    expect(screen.getByRole("button", { name: SAVE_AND_LEAVE_LABEL })).toBeTruthy();
  });

  it("does not treat a re-rendered same-id contributor as newly dirty mid-save", async () => {
    window.history.replaceState({}, "", APPEARANCE_PATH);
    const pending = deferred();
    const onSave = vi.fn(() => pending.promise);

    function PokingContributor() {
      const [, setPoke] = useState(0);
      return (
        <>
          <DraftContributor id="appearance" onSave={onSave} />
          <button type="button" onClick={() => setPoke((n) => n + 1)}>
            Poke appearance
          </button>
        </>
      );
    }

    render(
      <SettingsSaveProvider>
        <PokingContributor />
        <Link href={TERMINAL_PATH}>Terminal</Link>
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Edit appearance" }));
    fireEvent.click(screen.getByRole("link", { name: "Terminal" }));
    fireEvent.click(await screen.findByRole("button", { name: SAVE_AND_LEAVE_LABEL }));
    // Re-render the contributor while its save is pending: the registry
    // replaces the contributor object on every upsert, so an identity-based
    // "newly dirty" check would wrongly block leaving. Same id must not count.
    fireEvent.click(screen.getByRole("button", { name: "Poke appearance", hidden: true }));

    await act(async () => pending.resolve());

    await waitFor(() => expect(window.location.pathname).toBe(TERMINAL_PATH));
  });
});

describe("SettingsSaveProvider reset coordination", () => {
  it("settles navigation requested while a standalone reset is pending", async () => {
    window.history.replaceState({}, "", APPEARANCE_PATH);
    const pending = deferred();

    render(
      <SettingsSaveProvider>
        <DraftContributor id="appearance" onSave={vi.fn()} onDiscard={() => pending.promise} />
        <Link href={TERMINAL_PATH}>Terminal</Link>
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: RESET_LABEL }));
    fireEvent.click(screen.getByRole("link", { name: "Terminal" }));
    expect(await screen.findByRole("alertdialog")).toBeTruthy();

    await act(async () => pending.resolve());
    await waitFor(() => expect(window.location.pathname).toBe(TERMINAL_PATH));
  });

  it("preserves a newer edit made during an asynchronous reset", async () => {
    const pending = deferred();

    render(
      <SettingsSaveProvider>
        <DraftContributor id="profile" onSave={vi.fn()} onDiscard={() => pending.promise} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: RESET_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "Edit profile" }));

    await act(async () => pending.resolve());
    await waitFor(() => expect(screen.getByTestId(FLOATING_SAVE_TEST_ID)).toBeTruthy());
    expect(screen.getByRole("button", { name: SAVE_CHANGES_LABEL })).toBeTruthy();
  });
});
