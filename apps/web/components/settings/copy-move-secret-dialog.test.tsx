import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { SecretListItem } from "@/lib/types/http-secrets";
import {
  buildDefaultTargetName,
  CopyMoveSecretDialog,
  truncateUtf8Bytes,
} from "./copy-move-secret-dialog";

const { mockCopySecret, mockMoveSecret, mockUseDestinationNames, storeState } = vi.hoisted(() => {
  const storeState = {
    workspaces: {
      items: [
        { id: "ws-1", name: "Alpha" },
        { id: "ws-2", name: "Beta" },
      ],
      activeId: null,
    },
    setWorkspaces: vi.fn(),
  };
  return {
    mockCopySecret: vi.fn(),
    mockMoveSecret: vi.fn(),
    mockUseDestinationNames: vi.fn<
      (
        scope: string,
        workspaceId?: string,
      ) => { names: string[]; loaded: boolean; conflict: (name: string) => boolean }
    >(() => ({ names: [], loaded: true, conflict: () => false })),
    storeState,
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));
vi.mock("@/hooks/domains/settings/use-workspace-destinations", () => ({
  useWorkspaceDestinations: () => ({ loading: false, error: null, retry: vi.fn() }),
}));
vi.mock("@/hooks/domains/settings/use-secret-destination-names", () => ({
  useSecretDestinationNames: (scope: string, workspaceId?: string) =>
    mockUseDestinationNames(scope, workspaceId),
}));
vi.mock("@/lib/api/domains/secrets-api", () => ({
  copySecret: (...args: unknown[]) => mockCopySecret(...args),
  moveSecret: (...args: unknown[]) => mockMoveSecret(...args),
}));

const DEFAULT_GLOBAL_NAME = "API Key (from Global)";

const globalSecret: SecretListItem = {
  id: "secret-1",
  name: "API Key",
  scope: "global",
  has_value: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const workspaceSecret: SecretListItem = {
  ...globalSecret,
  id: "secret-2",
  name: "WS Key",
  scope: "workspace",
  workspace_id: "ws-1",
};

/** Renders the dialog with test fakes and returns the spies. */
function renderDialog(overrides: Partial<Parameters<typeof CopyMoveSecretDialog>[0]> = {}) {
  const onCompleted = vi.fn();
  const onClose = vi.fn();
  const view = render(
    <CopyMoveSecretDialog
      secret={globalSecret}
      originToken="Global"
      open
      onClose={onClose}
      onCompleted={onCompleted}
      {...overrides}
    />,
  );
  return { onCompleted, onClose, view };
}

afterEach(async () => {
  cleanup();
  mockCopySecret.mockReset();
  mockMoveSecret.mockReset();
  mockUseDestinationNames.mockReset();
  mockUseDestinationNames.mockReturnValue({ names: [], loaded: true, conflict: () => false });
  storeState.workspaces.items = [
    { id: "ws-1", name: "Alpha" },
    { id: "ws-2", name: "Beta" },
  ];
});

const TAKEN_NAME = "Taken";
const TAKEN_CONFLICT_MESSAGE = "A secret named Taken already exists in this destination.";

/** Whether the Copy/Move radio with the given name reports aria-checked. */
function radioChecked(name: RegExp): boolean {
  return screen.getByRole("radio", { name }).getAttribute("aria-checked") === "true";
}

/** Whether the target-name input reports aria-invalid. */
function inputInvalid(): boolean {
  return (
    (screen.getByLabelText("Name") as HTMLInputElement).getAttribute("aria-invalid") === "true"
  );
}

describe("CopyMoveSecretDialog", () => {
  it("pre-fills the name with the origin suffix and defaults to Copy", () => {
    renderDialog();
    expect(screen.getByRole("heading", { name: `Copy or move ${globalSecret.name}` })).toBeTruthy();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(DEFAULT_GLOBAL_NAME);
    expect(radioChecked(/^Copy/)).toBe(true);
    expect(screen.getByRole("button", { name: "Copy" })).toBeTruthy();
  });

  it("shows General as the default destination for a workspace source", () => {
    renderDialog({ secret: workspaceSecret, originToken: "Alpha" });
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("WS Key (from Alpha)");
    expect(screen.getByText("Global")).toBeTruthy();
  });

  it("resets the form when a different secret is targeted while mounted", async () => {
    // The dialog stays mounted between opens so Radix can restore focus to the
    // trigger on close; targeting a new secret must not leak the previous
    // mode/name/destination.
    const { view } = renderDialog({ secret: workspaceSecret, originToken: "Alpha" });
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Custom name" } });
    fireEvent.click(screen.getByRole("combobox", { name: "Destination" }));
    fireEvent.click(await screen.findByRole("option", { name: "Beta" }));

    view.rerender(
      <CopyMoveSecretDialog
        secret={globalSecret}
        originToken="Global"
        open
        onClose={vi.fn()}
        onCompleted={vi.fn()}
      />,
    );

    expect(radioChecked(/^Copy/)).toBe(true);
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(DEFAULT_GLOBAL_NAME);
    // Destination re-defaults for the new source (Global source: first workspace).
    expect(screen.getByRole("combobox", { name: "Destination" }).textContent).toContain("Alpha");
  });

  it("clears the form again when the same secret is reopened", () => {
    // Close then reopen the dialog for the same secret: the stale name/mode
    // from the previous session must not leak into the reopened dialog.
    const { view } = renderDialog();
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Custom name" } });

    view.rerender(
      <CopyMoveSecretDialog
        secret={globalSecret}
        originToken="Global"
        open={false}
        onClose={vi.fn()}
        onCompleted={vi.fn()}
      />,
    );
    view.rerender(
      <CopyMoveSecretDialog
        secret={globalSecret}
        originToken="Global"
        open
        onClose={vi.fn()}
        onCompleted={vi.fn()}
      />,
    );

    expect(radioChecked(/^Copy/)).toBe(true);
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(DEFAULT_GLOBAL_NAME);
  });

  it("switches the submit label to Move and shows the warning", () => {
    renderDialog();
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    expect(screen.getByRole("button", { name: "Move" })).toBeTruthy();
    expect(screen.getByText("The original secret will be removed from Global.")).toBeTruthy();
  });

  it("blocks submit on a conflicting target name with an invalid field", () => {
    mockUseDestinationNames.mockReturnValue({
      names: [TAKEN_NAME],
      loaded: true,
      conflict: (name: string) => name.trim() === TAKEN_NAME,
    });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: TAKEN_NAME } });
    expect(inputInvalid()).toBe(true);
    expect(screen.getByText(TAKEN_CONFLICT_MESSAGE)).toBeTruthy();
    expect((screen.getByRole("button", { name: "Copy" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("CopyMoveSecretDialog submit", () => {
  it("submits a copy with the destination workspace payload and calls onCompleted", async () => {
    mockCopySecret.mockResolvedValue({ id: "new-1", name: DEFAULT_GLOBAL_NAME });
    const { onCompleted } = renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(mockCopySecret).toHaveBeenCalledTimes(1));
    expect(mockCopySecret).toHaveBeenCalledWith(
      "secret-1",
      {
        target_scope: "workspace",
        target_workspace_id: "ws-1",
        name: DEFAULT_GLOBAL_NAME,
      },
      undefined,
    );
    await waitFor(() => expect(onCompleted).toHaveBeenCalled());
  });

  it("clears the workspace id when the destination switches to General", async () => {
    mockMoveSecret.mockResolvedValue({ id: "new-1", name: "moved" });
    renderDialog({ secret: workspaceSecret, originToken: "Alpha" });

    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() => expect(mockMoveSecret).toHaveBeenCalledTimes(1));
    const payload = mockMoveSecret.mock.calls[0][1] as Record<string, unknown>;
    expect(payload).toEqual({ target_scope: "global", name: "WS Key (from Alpha)" });
    expect(JSON.stringify(payload)).not.toContain('"name":null');
    expect(JSON.stringify(payload)).not.toContain("target_workspace_id");
    // A workspace-scoped source must carry its workspace id in the query.
    expect(mockMoveSecret.mock.calls[0][2]).toEqual({ workspaceId: "ws-1" });
  });

  it("surfaces a 409 conflict on the name field", async () => {
    mockCopySecret.mockRejectedValue(new ApiError("conflict", 409, {}));
    const { onCompleted } = renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: DEFAULT_GLOBAL_NAME } });
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() =>
      expect(
        screen.getByText(
          "A secret named API Key (from Global) already exists in this destination.",
        ),
      ).toBeTruthy(),
    );
    expect(onCompleted).not.toHaveBeenCalled();
  });
});

describe("CopyMoveSecretDialog conflict handling", () => {
  it("blocks an overlong target name with a localized byte-limit error", () => {
    renderDialog();
    const input = screen.getByLabelText("Name") as HTMLInputElement;
    // 101 bytes: exceeds the 100-byte UTF-8 backend limit without submitting.
    fireEvent.change(input, { target: { value: "x".repeat(101) } });
    expect(screen.getByText("Name must be at most 100 bytes.")).toBeTruthy();
    expect(inputInvalid()).toBe(true);
    expect((screen.getByRole("button", { name: "Copy" }) as HTMLButtonElement).disabled).toBe(true);

    // A multibyte name is counted in bytes, not characters.
    fireEvent.change(input, { target: { value: "é".repeat(51) } }); // 102 bytes
    expect(inputInvalid()).toBe(true);
    fireEvent.change(input, { target: { value: "é".repeat(50) } }); // 100 bytes: valid
    expect(inputInvalid()).toBe(false);
    expect((screen.getByRole("button", { name: "Copy" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("clears a 409 name conflict when the destination changes", async () => {
    mockMoveSecret.mockRejectedValue(new ApiError("conflict", 409, {}));
    renderDialog({ secret: workspaceSecret, originToken: "Alpha" });
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() => expect(screen.getByText(/already exists/)).toBeTruthy());
    expect((screen.getByRole("button", { name: "Move" }) as HTMLButtonElement).disabled).toBe(true);

    // The 409 is destination-specific: switching to a workspace where the name
    // is free must clear the error and re-enable submission.
    fireEvent.click(screen.getByRole("combobox", { name: "Destination" }));
    fireEvent.click(await screen.findByRole("option", { name: "Beta" }));

    // Prove the destination actually changed: the trigger must now display
    // Beta (not the initial General), otherwise the assertions below could
    // pass while the selection stayed on the original destination.
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Destination" }).textContent).toContain("Beta"),
    );
    await waitFor(() => expect(screen.queryByText(/already exists/)).toBeNull());
    expect((screen.getByRole("button", { name: "Move" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("shows a generic failure on 404 and never removes the source row", async () => {
    mockMoveSecret.mockRejectedValue(new ApiError("not found", 404, {}));
    const { onCompleted, onClose } = renderDialog({
      secret: workspaceSecret,
      originToken: "Alpha",
    });
    fireEvent.click(screen.getByRole("radio", { name: /^Move/ }));
    fireEvent.click(screen.getByRole("button", { name: "Move" }));

    await waitFor(() =>
      expect(screen.getByText("Could not copy or move the secret.")).toBeTruthy(),
    );
    // A 404 is deliberately ambiguous (missing source OR missing/unauthorized
    // destination): the dialog stays open, nothing is completed, and no row
    // removal callback fires.
    expect(onCompleted).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("disables submit synchronously when the selected workspace vanishes from the list", () => {
    const { view, onClose, onCompleted } = renderDialog();
    expect((screen.getByRole("button", { name: "Copy" }) as HTMLButtonElement).disabled).toBe(
      false,
    );

    storeState.workspaces.items = [];
    view.rerender(
      <CopyMoveSecretDialog
        secret={globalSecret}
        originToken="Global"
        open
        onClose={onClose}
        onCompleted={onCompleted}
      />,
    );

    // The validity check is synchronous: the stale workspace destination must
    // not be submittable even before the cleanup effect runs.
    expect((screen.getByRole("button", { name: "Copy" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("CopyMoveSecretDialog in-flight transfers", () => {
  it("ignores duplicate clicks while a transfer is in flight", async () => {
    const { promise, resolve } = Promise.withResolvers<unknown>();
    mockCopySecret.mockReturnValue(promise);
    renderDialog();
    const submit = screen.getByRole("button", { name: "Copy" });
    fireEvent.click(submit);
    fireEvent.click(submit);

    expect(mockCopySecret).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolve({ id: "new-1", name: DEFAULT_GLOBAL_NAME });
    });
  });

  it("discards a stale transfer completion after switching to another secret", async () => {
    // The dialog stays mounted between opens. Submit for secret A, then (while
    // the request is in flight) reopen the dialog for secret B: A's completion
    // must not fire onCompleted against B's dialog.
    const { promise, resolve } = Promise.withResolvers<unknown>();
    mockCopySecret.mockReturnValue(promise);
    const { view, onCompleted } = renderDialog({ secret: workspaceSecret, originToken: "Alpha" });
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));
    expect(mockCopySecret).toHaveBeenCalledTimes(1);

    // The close affordance is disabled while a transfer is in flight.
    expect((screen.getByRole("button", { name: "Close" }) as HTMLButtonElement).disabled).toBe(
      true,
    );

    view.rerender(
      <CopyMoveSecretDialog
        secret={globalSecret}
        originToken="Global"
        open
        onClose={vi.fn()}
        onCompleted={onCompleted}
      />,
    );
    await act(async () => {
      resolve({ id: "new-1", name: DEFAULT_GLOBAL_NAME });
    });

    expect(onCompleted).not.toHaveBeenCalled();
  });

  it("discards a stale completion when the same secret is reopened", async () => {
    // Submit for a secret, close the dialog (the X is disabled while busy but
    // Escape/overlay could still dismiss), then reopen the SAME secret: the
    // first session's completion must not fire against the reopened dialog.
    const { promise, resolve } = Promise.withResolvers<unknown>();
    mockCopySecret.mockReturnValue(promise);
    const { view, onCompleted } = renderDialog({ secret: workspaceSecret, originToken: "Alpha" });
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    view.rerender(
      <CopyMoveSecretDialog
        secret={workspaceSecret}
        originToken="Alpha"
        open={false}
        onClose={vi.fn()}
        onCompleted={onCompleted}
      />,
    );
    view.rerender(
      <CopyMoveSecretDialog
        secret={workspaceSecret}
        originToken="Alpha"
        open
        onClose={vi.fn()}
        onCompleted={onCompleted}
      />,
    );
    await act(async () => {
      resolve({ id: "new-1", name: DEFAULT_GLOBAL_NAME });
    });

    expect(onCompleted).not.toHaveBeenCalled();
  });
});

describe("truncateUtf8Bytes", () => {
  it("keeps ASCII under the limit unchanged", () => {
    expect(truncateUtf8Bytes("short", 100)).toBe("short");
  });

  it("truncates multibyte input to the byte limit", () => {
    const long = "é".repeat(60); // 120 bytes
    const truncated = truncateUtf8Bytes(long, 100);
    expect(new TextEncoder().encode(truncated).length).toBe(100);
    expect(truncated).toBe("é".repeat(50));
  });

  it("never splits astral-plane surrogate pairs", () => {
    const value = "🔐".repeat(40); // 160 bytes
    const truncated = truncateUtf8Bytes(value, 100);
    expect(truncated).toBe("🔐".repeat(25));
    expect(new TextEncoder().encode(truncated).length).toBe(100);
    expect(truncated.includes("\uFFFD")).toBe(false);
  });
});

describe("buildDefaultTargetName", () => {
  it("builds the locale-independent origin suffix within the byte limit", () => {
    const name = buildDefaultTargetName("API Key", "Global");
    expect(name).toBe(DEFAULT_GLOBAL_NAME);
    expect(new TextEncoder().encode(name).length).toBeLessThanOrEqual(100);
  });

  it("truncates an over-long source name so the copy can never fail validation", () => {
    const name = buildDefaultTargetName("K".repeat(120), "general");
    expect(new TextEncoder().encode(name).length).toBeLessThanOrEqual(100);
  });
});
