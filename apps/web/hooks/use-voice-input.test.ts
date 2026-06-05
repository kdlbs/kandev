import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// ── Hoisted mocks (defined before the modules they replace are evaluated) ──

const voicePrefs = vi.hoisted(() => ({
  value: {
    engine: "auto" as "auto" | "webSpeech" | "whisperWeb" | "whisperServer",
    language: "auto",
    mode: "toggle" as "toggle" | "hold",
    autoSend: false,
    whisperWebModel: "base" as "tiny" | "base" | "small",
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { userSettings: { voiceMode: typeof voicePrefs.value } }) => unknown,
  ) => selector({ userSettings: { voiceMode: voicePrefs.value } }),
}));

const transcribeAudio = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/voice-api", () => ({ transcribeAudio }));

// ── Mock SpeechRecognition ─────────────────────────────────────────────

type SpeechHandle = {
  start: () => void;
  stop: () => void;
  abort: () => void;
  onresult: ((ev: { resultIndex: number; results: unknown }) => void) | null;
  onerror: ((ev: { error: string }) => void) | null;
  onend: (() => void) | null;
  continuous: boolean;
  interimResults: boolean;
  maxAlternatives: number;
  lang: string;
  startCalls: number;
  stopCalls: number;
  abortCalls: number;
};

let recognitionInstance: SpeechHandle | null = null;

// Factory pattern instead of `class` so we can avoid aliasing `this` in the
// constructor (the lint rule disallows it) while still satisfying the
// `new ()` shape that useVoiceInput's `new Ctor()` calls.
function FakeSpeechRecognition() {
  const handle: SpeechHandle = {
    continuous: false,
    interimResults: false,
    maxAlternatives: 1,
    lang: "",
    onresult: null,
    onerror: null,
    onend: null,
    startCalls: 0,
    stopCalls: 0,
    abortCalls: 0,
    start() {
      handle.startCalls += 1;
    },
    stop() {
      handle.stopCalls += 1;
    },
    abort() {
      handle.abortCalls += 1;
    },
  };
  recognitionInstance = handle;
  return handle;
}

// Import after mocks so the module under test sees the mocked store.
import { useVoiceInput } from "./use-voice-input";

// ── Tests ───────────────────────────────────────────────────────────────

beforeEach(() => {
  voicePrefs.value = {
    engine: "auto",
    language: "auto",
    mode: "toggle",
    autoSend: false,
    whisperWebModel: "base",
  };
  recognitionInstance = null;
  transcribeAudio.mockReset();
  (window as unknown as { SpeechRecognition: unknown }).SpeechRecognition =
    FakeSpeechRecognition as unknown as new () => SpeechHandle;
  // MediaRecorder/getUserMedia not used in the auto→webSpeech path, but provide
  // a stub so capability detection sees audioCapture available too.
  (window as unknown as { MediaRecorder: { isTypeSupported: () => boolean } }).MediaRecorder = {
    isTypeSupported: () => true,
  };
  Object.defineProperty(global.navigator, "mediaDevices", {
    value: { getUserMedia: vi.fn() },
    configurable: true,
  });
});

afterEach(() => {
  delete (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition;
  delete (window as unknown as { webkitSpeechRecognition?: unknown }).webkitSpeechRecognition;
  delete (window as unknown as { MediaRecorder?: unknown }).MediaRecorder;
});

describe("useVoiceInput — Web Speech engine", () => {
  it("reports supported and resolves engine = webSpeech under the default auto preference", () => {
    const { result } = renderHook(() => useVoiceInput({ onTranscript: vi.fn() }));
    expect(result.current.supported).toBe(true);
    expect(result.current.engine).toBe("webSpeech");
  });

  it("transitions idle → recording on start() and emits the final transcript on stop()", async () => {
    const onTranscript = vi.fn();
    const { result } = renderHook(() => useVoiceInput({ onTranscript }));

    await act(async () => {
      await result.current.start();
    });
    expect(result.current.state).toBe("recording");
    expect(recognitionInstance?.startCalls).toBe(1);

    act(() => {
      recognitionInstance?.onresult?.({
        resultIndex: 0,
        results: {
          length: 1,
          0: { isFinal: true, length: 1, 0: { transcript: "hello world" } },
        } as unknown,
      });
      recognitionInstance?.onend?.();
    });

    await waitFor(() => {
      expect(onTranscript).toHaveBeenCalledWith("hello world");
      expect(result.current.state).toBe("idle");
    });
  });

  it("maps a not-allowed permission error to a permission-denied VoiceError", async () => {
    const onError = vi.fn();
    const { result } = renderHook(() => useVoiceInput({ onTranscript: vi.fn(), onError }));

    await act(async () => {
      await result.current.start();
    });
    act(() => {
      recognitionInstance?.onerror?.({ error: "not-allowed" });
    });

    expect(onError).toHaveBeenCalledWith({
      code: "permission-denied",
      message: "Microphone permission denied.",
    });
    expect(result.current.state).toBe("idle");
  });
});

describe("useVoiceInput — capability gating", () => {
  it("returns supported=false and engine=null when no engine is usable", () => {
    delete (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition;
    delete (window as unknown as { MediaRecorder?: unknown }).MediaRecorder;
    Object.defineProperty(global.navigator, "mediaDevices", { value: {}, configurable: true });

    const { result } = renderHook(() => useVoiceInput({ onTranscript: vi.fn() }));
    expect(result.current.supported).toBe(false);
    expect(result.current.engine).toBeNull();
  });

  it("disables the hook entirely when enabled=false", () => {
    const { result } = renderHook(() => useVoiceInput({ onTranscript: vi.fn(), enabled: false }));
    expect(result.current.supported).toBe(false);
    expect(result.current.engine).toBeNull();
  });
});

describe("useVoiceInput — language preference", () => {
  it("passes the pinned BCP-47 language to SpeechRecognition.lang", async () => {
    voicePrefs.value = { ...voicePrefs.value, language: "pt-PT" };
    const { result } = renderHook(() => useVoiceInput({ onTranscript: vi.fn() }));

    await act(async () => {
      await result.current.start();
    });
    expect(recognitionInstance?.lang).toBe("pt-PT");
  });

  it("falls back to navigator.language when 'auto'", async () => {
    voicePrefs.value = { ...voicePrefs.value, language: "auto" };
    Object.defineProperty(global.navigator, "language", { value: "fr-FR", configurable: true });
    const { result } = renderHook(() => useVoiceInput({ onTranscript: vi.fn() }));
    await act(async () => {
      await result.current.start();
    });
    expect(recognitionInstance?.lang).toBe("fr-FR");
  });
});
