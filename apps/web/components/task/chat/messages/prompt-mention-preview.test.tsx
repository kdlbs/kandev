import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { PromptMentionChip } from "./prompt-mention-components";

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));
const CONTENT = "Saved preview content";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function mount() {
  vi.useFakeTimers();
  render(
    <StateProvider
      initialState={{
        prompts: {
          items: [
            {
              id: "preview-test",
              name: "daily",
              content: CONTENT,
              builtin: false,
              created_at: "2026-10-21T00:00:00Z",
              updated_at: "2026-10-21T00:00:00Z",
            },
          ],
          loading: false,
          loaded: true,
        },
      }}
    >
      <PromptMentionChip name="daily" value="@daily" />
      <button type="button">Outside</button>
    </StateProvider>,
  );
  return screen.getByTestId("custom-prompt-mention");
}

async function advance(milliseconds = 1_000) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(milliseconds);
  });
}

function expectOpen(chip: HTMLElement, open: boolean) {
  expect(chip.getAttribute("aria-expanded")).toBe(String(open));
  expect(Boolean(screen.queryByText(CONTENT))).toBe(open);
}

// @covers AC-UI-PROMPT-ALIAS-001.1, AC-UI-PROMPT-ALIAS-002.4
describe("PromptMentionChip delayed-open dismissal", () => {
  it.each(["hover", "focus"])("keeps Escape dismissal ahead of pending %s", async (intent) => {
    const chip = mount();
    if (intent === "hover") fireEvent.pointerEnter(chip, { pointerType: "mouse" });
    else fireEvent.focus(chip);
    fireEvent.click(chip);
    expectOpen(chip, true);
    fireEvent.keyDown(document, { key: "Escape" });
    expectOpen(chip, false);
    await advance();
    expectOpen(chip, false);
  });

  it("keeps outside dismissal ahead of a pending hover", async () => {
    const chip = mount();
    fireEvent.pointerEnter(chip, { pointerType: "mouse" });
    fireEvent.click(chip);
    await advance(1);
    fireEvent.pointerDown(screen.getByRole("button", { name: "Outside" }), {
      pointerType: "mouse",
    });
    expectOpen(chip, false);
    await advance();
    expectOpen(chip, false);
  });

  it.each(["click", "Enter", " "])(
    "keeps %s toggle dismissal ahead of a pending focus",
    async (key) => {
      const chip = mount();
      fireEvent.focus(chip);
      const activate = () =>
        key === "click" ? fireEvent.click(chip) : fireEvent.keyDown(chip, { key });
      activate();
      expectOpen(chip, true);
      activate();
      expectOpen(chip, false);
      await advance();
      expectOpen(chip, false);
      activate();
      expectOpen(chip, true);
    },
  );
});

describe("PromptMentionChip fresh preview intent and lifecycle", () => {
  it.each(["hover", "focus"])(
    "opens on ordinary delayed %s and rearms after dismissal",
    async (intent) => {
      const chip = mount();
      const enter = () =>
        intent === "hover"
          ? fireEvent.pointerEnter(chip, { pointerType: "mouse" })
          : fireEvent.focus(chip);
      enter();
      expectOpen(chip, false);
      await advance(299);
      expectOpen(chip, false);
      await advance(1);
      expectOpen(chip, true);
      fireEvent.keyDown(document, { key: "Escape" });
      await advance();
      expectOpen(chip, false);
      if (intent === "hover") fireEvent.pointerLeave(chip, { pointerType: "mouse" });
      else fireEvent.blur(chip);
      enter();
      await advance();
      expectOpen(chip, true);
    },
  );

  it("toggles closed on a second pointer click without outside-dismiss double toggling", async () => {
    const chip = mount();
    fireEvent.click(chip);
    await advance(1);
    fireEvent.pointerDown(chip, { pointerType: "mouse" });
    fireEvent.click(chip);
    await advance();
    expectOpen(chip, false);
  });

  it("keeps click then Escape closed without a pending hover", async () => {
    const chip = mount();
    fireEvent.click(chip);
    expectOpen(chip, true);
    fireEvent.keyDown(document, { key: "Escape" });
    await advance();
    expectOpen(chip, false);
  });

  it("preserves pointer transit into content and content interaction", async () => {
    const chip = mount();
    fireEvent.pointerEnter(chip, { pointerType: "mouse" });
    await advance();
    const content = screen.getByText(CONTENT).closest('[data-slot="hover-card-content"]')!;
    fireEvent.pointerLeave(chip, { pointerType: "mouse" });
    fireEvent.pointerEnter(content, { pointerType: "mouse" });
    fireEvent.pointerDown(content, { pointerType: "mouse" });
    fireEvent.pointerUp(content, { pointerType: "mouse" });
    await advance();
    expectOpen(chip, true);
    fireEvent.pointerLeave(content, { pointerType: "mouse" });
    await advance();
    expectOpen(chip, false);
  });

  it("cancels a pending hover on leave and allows reentry", async () => {
    const chip = mount();
    fireEvent.pointerEnter(chip, { pointerType: "mouse" });
    fireEvent.pointerLeave(chip, { pointerType: "mouse" });
    await advance();
    expectOpen(chip, false);
    fireEvent.pointerEnter(chip, { pointerType: "mouse" });
    await advance();
    expectOpen(chip, true);
  });

  it("disposes pending opens on unmount without opening a replacement", async () => {
    const chip = mount();
    fireEvent.pointerEnter(chip, { pointerType: "mouse" });
    fireEvent.focus(chip);
    cleanup();
    const replacement = mount();
    await advance();
    expectOpen(replacement, false);
  });
});
