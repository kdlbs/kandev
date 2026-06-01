import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// ── Hoisted mocks (must be set before importing the component under test) ──

const voicePrefs = vi.hoisted(() => ({
  value: {
    enabled: true,
    engine: "auto" as "auto" | "webSpeech" | "whisperWeb" | "whisperServer",
    language: "auto",
    mode: "hold" as "hold" | "toggle",
    autoSend: false,
    whisperWebModel: "base" as "tiny" | "base" | "small",
  },
}));

const voiceInputResult = vi.hoisted(() => ({
  supported: true as boolean,
  state: "idle" as "idle" | "requesting" | "recording" | "processing",
  modelLoad: { state: "idle" as const, progress: 0 },
  start: vi.fn(async () => undefined),
  stop: vi.fn(async () => undefined),
  cancel: vi.fn(() => undefined),
  engine: "webSpeech" as const,
  error: null as null | { code: string; message: string },
}));

const coarsePointer = vi.hoisted(() => ({ value: false }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      userSettings: {
        voiceMode: typeof voicePrefs.value;
        keyboardShortcuts: Record<string, unknown>;
      };
    }) => unknown,
  ) =>
    selector({
      userSettings: { voiceMode: voicePrefs.value, keyboardShortcuts: {} },
    }),
}));

// Post-migration the component reads voiceMode/keyboardShortcuts from the TQ
// user-settings cache via useUserSettings() (useQuery). Mock it so tests don't
// need a QueryClientProvider; the hoisted `voicePrefs.value` stays the control.
vi.mock("@/hooks/domains/settings/use-user-settings", () => ({
  useUserSettings: () => ({
    data: { voiceMode: voicePrefs.value, keyboardShortcuts: {} },
    loaded: true,
    loading: false,
  }),
}));

vi.mock("@/hooks/use-voice-input", () => ({
  useVoiceInput: () => voiceInputResult,
}));

vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcut: () => undefined,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

// ── matchMedia stub honouring the hoisted `coarsePointer.value` flag ──

beforeEach(() => {
  voicePrefs.value = {
    enabled: true,
    engine: "auto",
    language: "auto",
    mode: "hold",
    autoSend: false,
    whisperWebModel: "base",
  };
  voiceInputResult.supported = true;
  voiceInputResult.state = "idle";
  voiceInputResult.modelLoad = { state: "idle", progress: 0 };
  voiceInputResult.start.mockClear();
  voiceInputResult.stop.mockClear();
  voiceInputResult.cancel.mockClear();
  coarsePointer.value = false;

  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    writable: true,
    value: (query: string) => ({
      matches: query.includes("coarse") ? coarsePointer.value : false,
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
});

afterEach(() => {
  cleanup();
});

// Re-import after the mocks so the module under test sees the mocked deps.
import { VoiceInputButton } from "./voice-input-button";

const BUTTON_TESTID = "voice-input-button";

function renderButton() {
  return render(<VoiceInputButton onTranscript={() => {}} onAutoSend={() => {}} />);
}

describe("VoiceInputButton — hold-mode pointer handlers", () => {
  it("captures the pointer on pointerdown and starts recording", () => {
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    const setCap = vi.fn();
    (button as unknown as { setPointerCapture: (id: number) => void }).setPointerCapture = setCap;

    fireEvent.pointerDown(button, { pointerId: 7 });

    expect(setCap).toHaveBeenCalledWith(7);
    expect(voiceInputResult.start).toHaveBeenCalledTimes(1);
  });

  it("releases the pointer and stops on pointerup", () => {
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    const releaseCap = vi.fn();
    (button as unknown as { hasPointerCapture: () => boolean }).hasPointerCapture = () => true;
    (button as unknown as { releasePointerCapture: (id: number) => void }).releasePointerCapture =
      releaseCap;

    fireEvent.pointerUp(button, { pointerId: 7 });

    expect(releaseCap).toHaveBeenCalledWith(7);
    expect(voiceInputResult.stop).toHaveBeenCalledTimes(1);
  });

  it("stops on pointercancel without triggering on pointerleave", () => {
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    (button as unknown as { hasPointerCapture: () => boolean }).hasPointerCapture = () => false;
    (button as unknown as { releasePointerCapture: (id: number) => void }).releasePointerCapture =
      () => undefined;

    // pointerleave used to abort recording. Confirm it no longer does.
    fireEvent.pointerLeave(button, { pointerId: 7 });
    expect(voiceInputResult.stop).not.toHaveBeenCalled();

    fireEvent.pointerCancel(button, { pointerId: 7 });
    expect(voiceInputResult.stop).toHaveBeenCalledTimes(1);
  });

  it("does not throw if setPointerCapture is not present on the element", () => {
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    // Some test/UA combinations strip setPointerCapture — handler must swallow.
    expect(() => fireEvent.pointerDown(button, { pointerId: 1 })).not.toThrow();
  });
});

describe("VoiceInputButton — coarse-pointer override", () => {
  it("reports stored hold mode but effective toggle when matchMedia(coarse) matches", () => {
    coarsePointer.value = true;
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    expect(button.getAttribute("data-mode")).toBe("hold");
    expect(button.getAttribute("data-effective-mode")).toBe("toggle");
  });

  it("does not attach hold pointer handlers in coarse-pointer toggle fallback", () => {
    coarsePointer.value = true;
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    fireEvent.pointerDown(button, { pointerId: 1 });
    fireEvent.pointerUp(button, { pointerId: 1 });
    // toggle handler is bound to onClick; pointer events alone should be inert.
    expect(voiceInputResult.start).not.toHaveBeenCalled();
    expect(voiceInputResult.stop).not.toHaveBeenCalled();

    // Click is what drives toggle.
    fireEvent.click(button);
    expect(voiceInputResult.start).toHaveBeenCalledTimes(1);
  });

  it("preserves hold behaviour on fine-pointer devices", () => {
    coarsePointer.value = false;
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    expect(button.getAttribute("data-effective-mode")).toBe("hold");
  });

  it("applies the 40px touch-target class on coarse-pointer devices", () => {
    coarsePointer.value = true;
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    expect(button.className).toContain("h-10");
    expect(button.className).toContain("w-10");
    expect(button.className).not.toContain("h-7");
  });
});

describe("VoiceInputButton — stored toggle mode is unaffected by pointer kind", () => {
  it("renders effective toggle and ignores pointer events when stored mode is toggle", () => {
    voicePrefs.value = { ...voicePrefs.value, mode: "toggle" };
    coarsePointer.value = false;
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    expect(button.getAttribute("data-mode")).toBe("toggle");
    expect(button.getAttribute("data-effective-mode")).toBe("toggle");

    // Pointer handlers must not be attached in toggle mode; pointerdown alone
    // should not start a recording. Click is what drives toggle.
    fireEvent.pointerDown(button, { pointerId: 4 });
    expect(voiceInputResult.start).not.toHaveBeenCalled();
    fireEvent.click(button);
    expect(voiceInputResult.start).toHaveBeenCalledTimes(1);
  });
});

describe("VoiceInputButton — recording-state stop transition", () => {
  it("invokes stop() on click when state is recording (toggle path)", () => {
    voicePrefs.value = { ...voicePrefs.value, mode: "toggle" };
    voiceInputResult.state = "recording";
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    fireEvent.click(button);
    expect(voiceInputResult.stop).toHaveBeenCalledTimes(1);
    expect(voiceInputResult.start).not.toHaveBeenCalled();
  });
});

describe("VoiceInputButton — disabled feature & unsupported fallback", () => {
  it("renders nothing when voice mode is disabled in settings", () => {
    voicePrefs.value = { ...voicePrefs.value, enabled: false };
    const { container } = renderButton();
    expect(container.firstChild).toBeNull();
  });

  it("renders the unsupported fallback button when no engine is available", () => {
    voiceInputResult.supported = false;
    renderButton();
    const button = screen.getByTestId(BUTTON_TESTID);
    expect(button.getAttribute("data-state")).toBe("unsupported");
    expect(button.getAttribute("aria-label")).toBe("Voice input unavailable");
  });
});
