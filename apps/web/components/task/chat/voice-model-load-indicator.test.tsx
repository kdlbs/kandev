import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { VoiceModelLoadIndicator } from "./voice-model-load-indicator";

const MODEL_LABEL = "Whisper Base";
const INDICATOR_TESTID = "voice-model-load-indicator";

function getIndicator() {
  return screen.getByTestId(INDICATOR_TESTID);
}

afterEach(() => {
  cleanup();
});

describe("VoiceModelLoadIndicator", () => {
  it("renders nothing when state is idle", () => {
    const { container } = render(
      <VoiceModelLoadIndicator state="idle" progress={0} modelLabel={MODEL_LABEL} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when state is ready", () => {
    const { container } = render(
      <VoiceModelLoadIndicator state="ready" progress={1} modelLabel={MODEL_LABEL} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders progress bar and percent text when loading", () => {
    render(<VoiceModelLoadIndicator state="loading" progress={0.42} modelLabel={MODEL_LABEL} />);
    const root = getIndicator();
    expect(root.getAttribute("data-state")).toBe("loading");
    expect(root.textContent).toContain("Downloading Whisper Base… 42%");
    expect(screen.getByRole("progressbar").getAttribute("aria-label")).toBe(
      "Downloading Whisper Base, 42 percent",
    );
  });

  it("renders an error label when state is error", () => {
    render(<VoiceModelLoadIndicator state="error" progress={0} modelLabel={MODEL_LABEL} />);
    const root = getIndicator();
    expect(root.getAttribute("data-state")).toBe("error");
    expect(root.textContent).toContain(`${MODEL_LABEL} failed to load`);
  });

  it("clamps percent to 0–100 when progress is out of range", () => {
    render(<VoiceModelLoadIndicator state="loading" progress={1.5} modelLabel={MODEL_LABEL} />);
    expect(getIndicator().textContent).toContain("100%");
    cleanup();

    render(<VoiceModelLoadIndicator state="loading" progress={-0.5} modelLabel={MODEL_LABEL} />);
    expect(getIndicator().textContent).toContain("0%");
    cleanup();

    render(
      <VoiceModelLoadIndicator state="loading" progress={Number.NaN} modelLabel={MODEL_LABEL} />,
    );
    expect(getIndicator().textContent).toContain("0%");
  });
});
