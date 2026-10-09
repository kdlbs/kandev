import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SSHIdentityFileField } from "./ssh-identity-file-field";
import { listSSHIdentities } from "@/lib/api/domains/ssh-api";

vi.mock("@/lib/api/domains/ssh-api", () => ({ listSSHIdentities: vi.fn() }));

const ED25519_PATH = "~/.ssh/id_ed25519";
const LOCKED_PATH = "~/.ssh/id_locked";
const CUSTOM_VALUE = "/opt/keys/deploy";

const discovered = {
  home_dir: "/home/user",
  identities: [
    {
      path: "/home/user/.ssh/id_ed25519",
      display_path: ED25519_PATH,
      key_type: "ssh-ed25519",
      encrypted: false,
      source: "ssh_dir" as const,
    },
    {
      path: "/home/user/.ssh/id_locked",
      display_path: LOCKED_PATH,
      key_type: "",
      encrypted: true,
      source: "ssh_dir" as const,
    },
  ],
};

function renderField(value: string, onChange = vi.fn()) {
  const view = render(
    <TooltipProvider>
      <SSHIdentityFileField value={value} isDirty={false} onChange={onChange} />
    </TooltipProvider>,
  );
  return { view, onChange };
}

const freeTextId = "ssh-input-identity-file";
const selectId = "ssh-identity-file-select";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SSHIdentityFileField", () => {
  it("offers discovered keys and reports the chosen path", async () => {
    vi.mocked(listSSHIdentities).mockResolvedValue(discovered);
    const { onChange } = renderField("");

    await waitFor(() => expect(screen.getByTestId(selectId)).toBeTruthy());
    fireEvent.click(screen.getByTestId(selectId));

    const labels = screen.getAllByRole("option").map((o) => o.textContent ?? "");
    expect(labels.some((l) => l.includes(ED25519_PATH))).toBe(true);

    fireEvent.click(screen.getByText(ED25519_PATH));
    expect(onChange).toHaveBeenCalledWith(ED25519_PATH);
  });

  // AC-…-001.4: an encrypted key cannot be used as a `file` identity source,
  // so offering it as selectable would only move the failure to Test
  // Connection. It stays visible, because hiding it reads as "key missing".
  it("shows a passphrase-protected key without letting it be selected", async () => {
    vi.mocked(listSSHIdentities).mockResolvedValue(discovered);
    const { onChange } = renderField("");

    await waitFor(() => expect(screen.getByTestId(selectId)).toBeTruthy());
    fireEvent.click(screen.getByTestId(selectId));

    const locked = screen
      .getAllByRole("option")
      .find((o) => (o.textContent ?? "").includes(LOCKED_PATH));
    expect(locked).toBeTruthy();
    expect(locked!.getAttribute("aria-disabled")).toBe("true");

    fireEvent.click(locked!);
    expect(onChange).not.toHaveBeenCalled();
  });

  // AC-…-001.6: the custom path is always reachable, whatever discovery found.
  it("reveals free-text entry when the custom path is chosen", async () => {
    vi.mocked(listSSHIdentities).mockResolvedValue(discovered);
    const { onChange } = renderField(ED25519_PATH);

    await waitFor(() => expect(screen.getByTestId(selectId)).toBeTruthy());
    expect(screen.queryByTestId(freeTextId)).toBeNull();

    fireEvent.click(screen.getByTestId(selectId));
    const custom = screen
      .getAllByRole("option")
      .find((o) => (o.textContent ?? "").toLowerCase().includes("custom"));
    expect(custom).toBeTruthy();
    fireEvent.click(custom!);

    const input = await screen.findByTestId(freeTextId);
    fireEvent.change(input, { target: { value: CUSTOM_VALUE } });
    expect(onChange).toHaveBeenCalledWith(CUSTOM_VALUE);
  });

  // AC-…-001.7: a value discovery did not return is a real configuration, not
  // a mistake to correct. It must survive and must not be repointed.
  it("keeps an undiscovered saved value as a custom path", async () => {
    vi.mocked(listSSHIdentities).mockResolvedValue(discovered);
    const { onChange } = renderField(CUSTOM_VALUE);

    const input = await screen.findByTestId(freeTextId);
    expect((input as HTMLInputElement).value).toBe(CUSTOM_VALUE);
    expect(onChange).not.toHaveBeenCalled();
  });

  // AC-…-001.5: discovery is admin-gated and best-effort. Losing it must cost
  // the user nothing, so the field degrades to exactly today's control.
  it.each([
    ["a rejected request", () => vi.mocked(listSSHIdentities).mockRejectedValue(new Error("403"))],
    [
      "an empty result",
      () =>
        vi.mocked(listSSHIdentities).mockResolvedValue({ home_dir: "/home/user", identities: [] }),
    ],
  ])("falls back to free-text entry on %s", async (_name, arrange) => {
    arrange();
    const { onChange } = renderField(ED25519_PATH);

    const input = await screen.findByTestId(freeTextId);
    expect((input as HTMLInputElement).value).toBe(ED25519_PATH);

    fireEvent.change(input, { target: { value: "~/.ssh/other" } });
    expect(onChange).toHaveBeenCalledWith("~/.ssh/other");
  });

  it("does not query discovery more than once per mount", async () => {
    vi.mocked(listSSHIdentities).mockResolvedValue(discovered);
    renderField("");

    await waitFor(() => expect(screen.getByTestId(selectId)).toBeTruthy());
    expect(vi.mocked(listSSHIdentities)).toHaveBeenCalledTimes(1);
  });
});
