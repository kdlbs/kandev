import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  listPrompts: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  listPrompts: mocks.listPrompts,
}));

import { StateProvider } from "@/components/state-provider";
import { useCustomPrompts } from "./use-custom-prompts";

function PromptEditorLoaders() {
  const first = useCustomPrompts();
  const second = useCustomPrompts();

  return (
    <output data-testid="prompt-editor-loaders">
      {String(first.loading)}:{String(second.loading)}
    </output>
  );
}

describe("useCustomPrompts", () => {
  afterEach(() => {
    cleanup();
    mocks.listPrompts.mockReset();
  });

  it("deduplicates the prompt request for two mounted editors", async () => {
    let resolveRequest!: (response: { prompts: [] }) => void;
    mocks.listPrompts.mockReturnValue(
      new Promise<{ prompts: [] }>((resolve) => {
        resolveRequest = resolve;
      }),
    );

    render(
      <StateProvider>
        <PromptEditorLoaders />
      </StateProvider>,
    );

    await waitFor(() => expect(mocks.listPrompts).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByTestId("prompt-editor-loaders").textContent).toBe("true:true"),
    );

    resolveRequest({ prompts: [] });

    await waitFor(() =>
      expect(screen.getByTestId("prompt-editor-loaders").textContent).toBe("false:false"),
    );
  });
});
