import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { createDefaultUserSettings } from "@/lib/ssr/user-settings";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import { SettingsSaveProvider, useSettingsSaveCoordinator } from "./settings-save-provider";
import { TerminalSettings } from "./terminal-settings";

const CUSTOM_INPUT_ID = "terminal-font-custom-input";
const OBSERVED_FAMILY_ID = "observed-family";
const PRESET = '"JetBrains Mono", "Fira Code", Menlo, Consolas, monospace';

type Patch = {
  body: Record<string, unknown>;
  resolve: (response: Response) => void;
};
let patches: Patch[];

function SaveObserver() {
  const coordinator = useSettingsSaveCoordinator();
  const settings = useAppStore((state) => state.userSettings);
  return (
    <>
      <output data-testid="observed-dirty">{String(coordinator.hasDirty)}</output>
      <output data-testid="observed-contributor">
        {String(
          coordinator.contributorStates.find((entry) => entry.id === "general-terminal")?.isDirty,
        )}
      </output>
      <output data-testid="observed-family">{settings.terminalFontFamily}</output>
      <output data-testid="observed-size">{settings.terminalFontSize}</output>
    </>
  );
}

function mountTerminal(terminalFontFamily: string | null = "OriginalFont") {
  render(
    <StateProvider
      initialState={{
        userSettings: {
          ...createDefaultUserSettings(),
          terminalFontFamily,
          terminalFontSize: 13,
          loaded: true,
        },
      }}
    >
      <ToastProvider>
        <SettingsSaveProvider>
          <TerminalSettings />
          <SaveObserver />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

function customInput() {
  return screen.getByTestId(CUSTOM_INPUT_ID) as HTMLInputElement;
}

function changeFont(value: string, blur = false) {
  const input = customInput();
  act(() => input.focus());
  fireEvent.change(input, { target: { value } });
  if (blur) fireEvent.blur(input);
}

async function startSave(expectedCount: number, name = "Save changes") {
  fireEvent.click(screen.getByRole("button", { name }));
  await waitFor(() => expect(patches).toHaveLength(expectedCount));
}

async function acknowledge(index: number, status = 200) {
  await act(async () => {
    patches[index].resolve(
      new Response(JSON.stringify({ settings: patches[index].body }), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
}

function expectDirty(value: boolean) {
  expect(screen.getByTestId("observed-dirty").textContent).toBe(String(value));
  expect(screen.getByTestId("observed-contributor").textContent).toBe(String(value));
}

async function selectFont(name: string) {
  fireEvent.click(screen.getByTestId("terminal-font-select"));
  fireEvent.click(await screen.findByRole("option", { name }));
}

async function reset() {
  fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  await waitFor(() => expectDirty(false));
}

beforeEach(() => {
  patches = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init: RequestInit) => {
      if (!url.endsWith("/api/v1/user/settings") || init.method !== "PATCH") {
        throw new Error(`Unexpected fetch: ${init.method} ${url}`);
      }
      return new Promise<Response>((resolve) => {
        patches.push({ body: JSON.parse(String(init.body)), resolve });
      });
    }),
  );
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const patch of patches) patch.resolve(new Response("{}", { status: 200 }));
  });
  clearNavigationBlockerForTests();
  vi.unstubAllGlobals();
});

describe("terminal custom-font save registration", () => {
  // @covers AC-UI-SETTINGS-MANUAL-SAVE-001.1, AC-UI-SETTINGS-MANUAL-SAVE-001.2, AC-UI-SETTINGS-MANUAL-SAVE-001.4
  it("keeps newer unblurred custom font dirty and saves it explicitly", async () => {
    mountTerminal();
    changeFont("SubmittedFont", true);
    expect(patches).toHaveLength(0);
    await startSave(1);
    expect(patches[0].body.terminal_font_family).toBe("SubmittedFont");
    changeFont("NewerFont");
    expect(document.activeElement).toBe(customInput());
    await acknowledge(0);
    expect(screen.getByTestId(OBSERVED_FAMILY_ID).textContent).toBe("SubmittedFont");
    expect(customInput().value).toBe("NewerFont");
    expect(document.activeElement).toBe(customInput());
    expectDirty(true);
    expect(
      (screen.getByRole("button", { name: "Save changes" }) as HTMLButtonElement).disabled,
    ).toBe(false);
    await startSave(2);
    expect(patches[1].body.terminal_font_family).toBe("NewerFont");
    await acknowledge(1);
    expect(screen.getByTestId(OBSERVED_FAMILY_ID).textContent).toBe("NewerFont");
    expectDirty(false);
  });

  it("saves an ordinary custom font without newer edits", async () => {
    mountTerminal();
    changeFont("SubmittedFont", true);
    expect(patches).toHaveLength(0);
    await startSave(1);
    expect(patches[0].body.terminal_font_family).toBe("SubmittedFont");
    await acknowledge(0);
    expect(customInput().value).toBe("SubmittedFont");
    expect(screen.getByTestId(OBSERVED_FAMILY_ID).textContent).toBe("SubmittedFont");
    expectDirty(false);
  });

  it("keeps newer parent-managed font size dirty after font ACK", async () => {
    mountTerminal();
    changeFont("SubmittedFont", true);
    await startSave(1);
    fireEvent.change(screen.getByTestId("terminal-font-size-input"), { target: { value: "18" } });
    await acknowledge(0);
    expect(screen.getByTestId("observed-size").textContent).toBe("13");
    expect((screen.getByTestId("terminal-font-size-input") as HTMLInputElement).value).toBe("18");
    expectDirty(true);
    expect(
      (screen.getByRole("button", { name: "Save changes" }) as HTMLButtonElement).disabled,
    ).toBe(false);
    await startSave(2);
    expect(patches[1].body.terminal_font_size).toBe(18);
    await acknowledge(1);
    expect(screen.getByTestId("observed-size").textContent).toBe("18");
    expectDirty(false);
  });
});

describe("custom font acceptance boundaries", () => {
  it.each(["", "   "])("retains the accepted family for blank custom text %j", async (raw) => {
    mountTerminal();
    changeFont(raw, true);
    expect(customInput().value).toBe(raw);
    expectDirty(true);
    expect(patches).toHaveLength(0);
    await startSave(1);
    expect(patches[0].body.terminal_font_family).toBe("OriginalFont");
    await acknowledge(0);
    expect(customInput().value).toBe("OriginalFont");
    expectDirty(false);
  });

  it.each(["blur", "Enter"])("canonicalizes a typed preset only on %s", async (boundary) => {
    mountTerminal();
    changeFont(PRESET);
    expect(customInput().value).toBe(PRESET);
    expectDirty(true);
    if (boundary === "blur") fireEvent.blur(customInput());
    else fireEvent.keyDown(customInput(), { key: "Enter" });
    expect(screen.queryByTestId(CUSTOM_INPUT_ID)).toBeNull();
    expect(screen.getByTestId("terminal-font-select").textContent).toBe("JetBrains Mono");
    expect(patches).toHaveLength(0);
    await startSave(1);
    expect(patches[0].body.terminal_font_family).toBe(PRESET);
    await acknowledge(0);
    expectDirty(false);
  });

  it("trims only captured custom text and preserves newer raw whitespace", async () => {
    mountTerminal();
    changeFont("  SubmittedFont  ");
    await startSave(1);
    expect(patches[0].body.terminal_font_family).toBe("SubmittedFont");
    changeFont("  NewerFont  ");
    await acknowledge(0);
    expect(customInput().value).toBe("  NewerFont  ");
    expectDirty(true);
    await startSave(2);
    expect(patches[1].body.terminal_font_family).toBe("NewerFont");
    await acknowledge(1);
    expect(customInput().value).toBe("NewerFont");
    expectDirty(false);
  });
});

describe("custom font reset and retry", () => {
  it("resets the newer raw editor to the last acknowledged family", async () => {
    mountTerminal();
    changeFont("SubmittedFont", true);
    await startSave(1);
    changeFont("NewerFont");
    await acknowledge(0);
    await reset();
    expect(customInput().value).toBe("SubmittedFont");
    expect(patches).toHaveLength(1);
  });

  it("resets Custom mode when the accepted preset family is unchanged", async () => {
    mountTerminal(PRESET);
    await selectFont("Custom...");
    expect(customInput().value).toBe("");
    expectDirty(false);
    fireEvent.change(screen.getByTestId("terminal-font-size-input"), { target: { value: "18" } });
    await reset();
    expect(screen.queryByTestId(CUSTOM_INPUT_ID)).toBeNull();
    expect(screen.getByTestId("terminal-font-select").textContent).toBe("JetBrains Mono");
    expect(patches).toHaveLength(0);
  });

  it.each([
    ["JetBrains Mono", PRESET],
    ["Default (Menlo / Monaco)", ""],
  ])("explicitly stages the %s selection", async (name, family) => {
    mountTerminal();
    changeFont("NewerFont");
    await selectFont(name);
    expect(screen.queryByTestId(CUSTOM_INPUT_ID)).toBeNull();
    expect(patches).toHaveLength(0);
    await startSave(1);
    expect(patches[0].body.terminal_font_family).toBe(family);
    await acknowledge(0);
    expectDirty(false);
  });

  it("clears dirty state when raw input returns to its baseline", () => {
    mountTerminal();
    changeFont("NewerFont");
    expectDirty(true);
    changeFont("OriginalFont");
    expectDirty(false);
    expect(patches).toHaveLength(0);
  });

  it("keeps raw input registered after failure and retries explicitly", async () => {
    mountTerminal();
    changeFont("  RetryFont  ");
    await startSave(1);
    await acknowledge(0, 500);
    expect(customInput().value).toBe("  RetryFont  ");
    expect(screen.getByTestId(OBSERVED_FAMILY_ID).textContent).toBe("OriginalFont");
    expectDirty(true);
    await startSave(2, "Retry save");
    expect(patches[1].body.terminal_font_family).toBe("RetryFont");
    await acknowledge(1);
    expect(customInput().value).toBe("RetryFont");
    expectDirty(false);
  });
});
