import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { SettingsTargetProvider } from "@/components/settings/settings-target-provider";
import { ApiTokens } from "./api-tokens";

const api = vi.hoisted(() => ({ listTokens: vi.fn(), mintToken: vi.fn(), revokeToken: vi.fn() }));
vi.mock("@/lib/api/domains/auth-api", () => api);

const NAME_FIELD = "api-tokens-name";

function openDialog() {
  fireEvent.click(screen.getByTestId("api-tokens-create"));
  return screen.getByRole("dialog");
}

async function mint(name: string) {
  fireEvent.change(screen.getByTestId(NAME_FIELD), { target: { value: name } });
  fireEvent.click(screen.getByTestId("api-tokens-mint-submit"));
  return screen.findByTestId("api-tokens-raw-value");
}

beforeEach(async () => {
  api.listTokens.mockResolvedValue({ tokens: [] });
  api.mintToken.mockImplementation(async ({ name }: { name: string }) => ({
    token: `dummy-${name}`,
    record: { id: name, user_id: "user", name, created_at: "2026-01-01T00:00:00Z" },
  }));
  render(
    <SettingsTargetProvider>
      <SettingsSaveProvider>
        <TooltipProvider>
          <ApiTokens />
        </TooltipProvider>
      </SettingsSaveProvider>
    </SettingsTargetProvider>,
  );
  await screen.findByTestId("api-tokens-empty");
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("API token creation", () => {
  it("shows the first result and closes it with Done", async () => {
    openDialog();
    expect(((await mint("first")) as HTMLInputElement).value).toBe("dummy-first");
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("starts a fresh second creation after Done", async () => {
    openDialog();
    await mint("first");
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    openDialog();
    expect(screen.queryByTestId("api-tokens-raw-value")).toBeNull();
    expect((screen.getByTestId(NAME_FIELD) as HTMLInputElement).value).toBe("");
    expect(((await mint("second")) as HTMLInputElement).value).toBe("dummy-second");
    expect(api.mintToken).toHaveBeenNthCalledWith(2, { name: "second" });
  });

  it("clears a creation error when Cancel closes the form", async () => {
    api.mintToken.mockRejectedValueOnce(new Error("fixture failure"));
    openDialog();
    fireEvent.change(screen.getByTestId(NAME_FIELD), { target: { value: "failed" } });
    fireEvent.click(screen.getByTestId("api-tokens-mint-submit"));
    await screen.findByTestId("api-tokens-mint-error");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    openDialog();
    expect(screen.queryByTestId("api-tokens-mint-error")).toBeNull();
    expect((screen.getByTestId(NAME_FIELD) as HTMLInputElement).value).toBe("");
  });

  it("starts a new result without the previous copied indication", async () => {
    vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    openDialog();
    await mint("first");
    fireEvent.click(screen.getByTestId("api-tokens-copy"));
    await waitFor(() =>
      expect(
        screen.getByTestId("api-tokens-copy").querySelector(".tabler-icon-check"),
      ).not.toBeNull(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    openDialog();
    await mint("second");
    expect(screen.getByTestId("api-tokens-copy").querySelector(".tabler-icon-copy")).not.toBeNull();
  });

  it.each(["Cancel", "Close", "Escape"])("clears a dismissed form via %s", async (method) => {
    const dialog = openDialog();
    fireEvent.change(screen.getByTestId(NAME_FIELD), { target: { value: "discard me" } });
    if (method === "Escape") fireEvent.keyDown(document, { key: "Escape" });
    else fireEvent.click(within(dialog).getByRole("button", { name: method }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    openDialog();
    expect((screen.getByTestId(NAME_FIELD) as HTMLInputElement).value).toBe("");
  });
});
