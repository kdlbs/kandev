import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { AnchoredLastPromptBar } from "./anchored-last-prompt-bar";
import type { StoreApi } from "zustand";

function StoreCapture({ onStore }: { onStore: (store: StoreApi<AppState>) => void }) {
  onStore(useAppStoreApi());
  return null;
}

afterEach(() => {
  cleanup();
});

describe("AnchoredLastPromptBar prompt mention reactivity", () => {
  it("updates recognition and preview content from mounted store changes", () => {
    let store: StoreApi<AppState> | null = null;
    render(
      <StateProvider initialState={{ prompts: { items: [], loaded: true, loading: false } }}>
        <StoreCapture onStore={(nextStore) => (store = nextStore)} />
        <TooltipProvider delayDuration={0}>
          <AnchoredLastPromptBar
            promptText="Review @daily"
            isVisible={true}
            onScrollUp={() => {}}
          />
        </TooltipProvider>
      </StateProvider>,
    );

    expect(screen.queryByTestId("custom-prompt-mention")).toBeNull();

    act(() => {
      store?.getState().setPrompts([
        {
          id: "daily",
          name: "daily",
          content: "Initial prompt content",
          builtin: false,
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:00:00.000Z",
        },
      ]);
    });

    const mention = screen.getByTestId("custom-prompt-mention");
    expect(mention.getAttribute("data-prompt-name")).toBe("daily");
    fireEvent.click(mention);
    expect(screen.getByText("Initial prompt content")).toBeTruthy();

    act(() => {
      store?.getState().setPrompts([
        {
          id: "daily",
          name: "daily",
          content: "Updated prompt content",
          builtin: false,
          created_at: "2026-01-01T00:00:00.000Z",
          updated_at: "2026-01-01T00:01:00.000Z",
        },
      ]);
    });
    expect(screen.getByText("Updated prompt content")).toBeTruthy();
    expect(screen.queryByText("Initial prompt content")).toBeNull();
    expect(screen.getByTestId("custom-prompt-mention").getAttribute("aria-expanded")).toBe("true");
  });
});
