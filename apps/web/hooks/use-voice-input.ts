"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { transcribeAudio } from "@/lib/api/domains/voice-api";
import { detectVoiceCapabilities, resolveActiveEngine } from "@/lib/voice/capabilities";
import { WhisperWebClient, type WhisperWebProgress } from "@/lib/voice/whisper-web-client";
import { useAppStore } from "@/components/state-provider";
import type { VoiceInputEngine, WhisperWebModelSize } from "@/lib/types/http-voice";

// ── Public types ────────────────────────────────────────────────────────

export type VoiceInputState = "idle" | "requesting" | "recording" | "processing";

export type VoiceErrorCode =
  | "permission-denied"
  | "no-speech"
  | "not-configured"
  | "network"
  | "unsupported"
  | "model-load"
  | "unknown";

export type VoiceError = { code: VoiceErrorCode; message: string };

export type VoiceModelLoadState = {
  state: "idle" | "loading" | "ready" | "error";
  progress: number;
};

export type UseVoiceInputOptions = {
  onTranscript: (text: string) => void;
  onError?: (error: VoiceError) => void;
  /** Set false to disable the hook entirely (e.g. for read-only contexts). */
  enabled?: boolean;
};

export type UseVoiceInputResult = {
  supported: boolean;
  engine: Exclude<VoiceInputEngine, "auto"> | null;
  state: VoiceInputState;
  error: VoiceError | null;
  modelLoad: VoiceModelLoadState;
  start: () => Promise<void>;
  stop: () => Promise<void>;
  cancel: () => void;
};

// ── Web Speech typings (DOM lib doesn't ship them) ─────────────────────

type SpeechAlt = { transcript: string };
type SpeechResult = { isFinal: boolean; 0: SpeechAlt; length: number };
type SpeechResultList = { length: number; [index: number]: SpeechResult };
type SpeechResultEvent = { resultIndex: number; results: SpeechResultList };
type SpeechErrorEvent = { error: string; message?: string };
type SpeechRecognitionInstance = {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  maxAlternatives: number;
  start: () => void;
  stop: () => void;
  abort: () => void;
  onresult: ((ev: SpeechResultEvent) => void) | null;
  onerror: ((ev: SpeechErrorEvent) => void) | null;
  onend: (() => void) | null;
};

type SpeechCtor = new () => SpeechRecognitionInstance;

function createSpeechRecognition(): SpeechRecognitionInstance | null {
  if (typeof window === "undefined") return null;
  const w = window as Window & {
    SpeechRecognition?: SpeechCtor;
    webkitSpeechRecognition?: SpeechCtor;
  };
  const Ctor = w.SpeechRecognition ?? w.webkitSpeechRecognition;
  return Ctor ? new Ctor() : null;
}

// ── Error mappers ───────────────────────────────────────────────────────

function mapSpeechError(code: string): VoiceError {
  if (code === "not-allowed" || code === "service-not-allowed") {
    return { code: "permission-denied", message: "Microphone permission denied." };
  }
  if (code === "no-speech") return { code: "no-speech", message: "No speech detected. Try again." };
  if (code === "network") {
    return { code: "network", message: "Voice recognition lost network connection." };
  }
  if (code === "audio-capture") return { code: "unknown", message: "No microphone was found." };
  return { code: "unknown", message: `Voice recognition error: ${code}` };
}

function mapMicError(err: unknown): VoiceError {
  if (err && typeof err === "object" && "name" in err) {
    const name = (err as { name: string }).name;
    if (name === "NotAllowedError" || name === "SecurityError") {
      return { code: "permission-denied", message: "Microphone permission denied." };
    }
    if (name === "NotFoundError" || name === "OverconstrainedError") {
      return { code: "unknown", message: "No microphone was found." };
    }
  }
  return { code: "unknown", message: "Failed to start recording." };
}

function mapTranscribeError(err: unknown): VoiceError {
  if (err instanceof ApiError && err.status === 503) {
    return {
      code: "not-configured",
      message:
        "Server-side transcription isn't configured. Pick Web Speech or Whisper Web in Voice Mode settings.",
    };
  }
  return { code: "network", message: "Transcription failed. Please try again." };
}

function whisperErrorMessage(err: unknown): VoiceError {
  const message = err instanceof Error ? err.message : "Whisper Web failed to transcribe.";
  return { code: "model-load", message };
}

function resolveLang(preference: string): string {
  if (preference && preference !== "auto") return preference;
  return typeof navigator !== "undefined" ? navigator.language : "en-US";
}

function resolveWhisperLang(preference: string): string | undefined {
  if (!preference || preference === "auto") return undefined;
  // Whisper's tokenizer only knows ISO 639-1 two-letter codes ("en", "pt").
  // The settings UI stores BCP-47 ("en-US", "pt-BR") so we can render
  // human-friendly variant names — strip the region suffix here so the hint
  // isn't silently dropped by the pipeline (which would then auto-detect and
  // potentially pick the wrong dialect).
  const dash = preference.indexOf("-");
  return dash > 0 ? preference.slice(0, dash).toLowerCase() : preference.toLowerCase();
}

// ── MediaRecorder capture primitive ─────────────────────────────────────

function pickRecorderMime(): { mime: string; ext: string } {
  if (typeof window === "undefined" || typeof window.MediaRecorder === "undefined") {
    return { mime: "", ext: "webm" };
  }
  const candidates: Array<{ mime: string; ext: string }> = [
    { mime: "audio/webm;codecs=opus", ext: "webm" },
    { mime: "audio/webm", ext: "webm" },
    { mime: "audio/mp4", ext: "m4a" },
    { mime: "audio/ogg;codecs=opus", ext: "ogg" },
    { mime: "audio/wav", ext: "wav" },
  ];
  for (const c of candidates) {
    if (window.MediaRecorder.isTypeSupported(c.mime)) return c;
  }
  return { mime: "", ext: "webm" };
}

type CaptureHandle = {
  stream: MediaStream;
  recorder: MediaRecorder;
  chunks: Blob[];
  mime: string;
  ext: string;
};

async function startCapture(): Promise<CaptureHandle> {
  const { mime, ext } = pickRecorderMime();
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  const recorder = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined);
  const chunks: Blob[] = [];
  recorder.addEventListener("dataavailable", (e) => {
    if (e.data && e.data.size > 0) chunks.push(e.data);
  });
  recorder.start();
  return { stream, recorder, chunks, mime, ext };
}

function teardownCapture(handle: CaptureHandle | null) {
  if (!handle) return;
  for (const t of handle.stream.getTracks()) t.stop();
}

function stopCapture(handle: CaptureHandle): Promise<Blob | null> {
  return new Promise((resolve) => {
    if (handle.recorder.state === "inactive") {
      teardownCapture(handle);
      resolve(null);
      return;
    }
    handle.recorder.addEventListener(
      "stop",
      () => {
        const type = handle.recorder.mimeType || handle.mime || "audio/webm";
        const blob = handle.chunks.length > 0 ? new Blob(handle.chunks, { type }) : null;
        teardownCapture(handle);
        resolve(blob);
      },
      { once: true },
    );
    handle.recorder.stop();
  });
}

// ── Driver refs ─────────────────────────────────────────────────────────

type ActiveDriverRef =
  | { kind: "webSpeech"; recognition: SpeechRecognitionInstance }
  | { kind: "capture"; handle: CaptureHandle; engine: "whisperWeb" | "whisperServer" }
  | null;

type DriverRefBox = { current: ActiveDriverRef };
type WhisperRefBox = { current: WhisperWebClient | null };

function abortDriver(ref: DriverRefBox) {
  const driver = ref.current;
  if (!driver) return;
  if (driver.kind === "webSpeech") {
    // Detach callbacks before aborting so the trailing onerror/onend events
    // that some browsers fire after .abort() don't sneak through and mutate
    // hook state that the caller (cancel()) just reset.
    driver.recognition.onresult = null;
    driver.recognition.onerror = null;
    driver.recognition.onend = null;
    driver.recognition.abort();
  } else teardownCapture(driver.handle);
  ref.current = null;
}

// ── Web Speech driver ───────────────────────────────────────────────────

type WebSpeechHandlers = {
  setState: (s: VoiceInputState) => void;
  driverRef: DriverRefBox;
  emitError: (e: VoiceError) => void;
  onTranscriptRef: { current: (text: string) => void };
  lang: string;
};

function runWebSpeech(h: WebSpeechHandlers): void {
  const recognition = createSpeechRecognition();
  if (!recognition) {
    h.emitError({ code: "unsupported", message: "Voice recognition is not supported." });
    return;
  }
  const transcripts: string[] = [];
  recognition.continuous = true;
  recognition.interimResults = false;
  recognition.maxAlternatives = 1;
  recognition.lang = h.lang;
  recognition.onresult = (ev) => {
    for (let i = ev.resultIndex; i < ev.results.length; i++) {
      const r = ev.results[i];
      if (r.isFinal && r[0]?.transcript) transcripts.push(r[0].transcript.trim());
    }
  };
  recognition.onerror = (ev) => h.emitError(mapSpeechError(ev.error));
  recognition.onend = () => {
    h.driverRef.current = null;
    h.setState("idle");
    const joined = transcripts.join(" ").trim();
    if (joined) h.onTranscriptRef.current(joined);
  };
  try {
    recognition.start();
    h.driverRef.current = { kind: "webSpeech", recognition };
    h.setState("recording");
  } catch {
    h.emitError({ code: "unknown", message: "Failed to start voice recognition." });
  }
}

// ── Capture engines (whisperWeb + whisperServer) ───────────────────────

type CaptureHandlers = {
  setState: (s: VoiceInputState) => void;
  emitError: (e: VoiceError) => void;
  driverRef: DriverRefBox;
};

async function beginCapture(
  which: "whisperWeb" | "whisperServer",
  h: CaptureHandlers,
): Promise<void> {
  h.setState("requesting");
  try {
    const handle = await startCapture();
    h.driverRef.current = { kind: "capture", handle, engine: which };
    h.setState("recording");
  } catch (err) {
    h.emitError(mapMicError(err));
  }
}

type FinishCaptureHandlers = {
  driverRef: DriverRefBox;
  whisperRef: WhisperRefBox;
  setState: (s: VoiceInputState) => void;
  setModelLoad: (next: VoiceModelLoadState) => void;
  emitError: (e: VoiceError) => void;
  onTranscriptRef: { current: (text: string) => void };
  whisperModel: WhisperWebModelSize;
  language: string;
};

async function finishCapture(h: FinishCaptureHandlers): Promise<void> {
  const driver = h.driverRef.current;
  if (!driver || driver.kind !== "capture") return;
  h.setState("processing");
  const blob = await stopCapture(driver.handle);
  h.driverRef.current = null;
  if (!blob) {
    h.setState("idle");
    return;
  }
  try {
    const text =
      driver.engine === "whisperServer"
        ? await transcribeViaServer(blob, driver.handle.ext)
        : await transcribeViaWhisperWeb(blob, h);
    if (text) h.onTranscriptRef.current(text);
    h.setState("idle");
  } catch (err) {
    if (driver.engine === "whisperServer") h.emitError(mapTranscribeError(err));
    else h.emitError(whisperErrorMessage(err));
  }
}

async function transcribeViaServer(blob: Blob, ext: string): Promise<string> {
  const result = await transcribeAudio(blob, `recording.${ext}`);
  return result.text.trim();
}

async function transcribeViaWhisperWeb(blob: Blob, h: FinishCaptureHandlers): Promise<string> {
  const client = await ensureWhisperClient(h);
  const text = await client.transcribe(blob, resolveWhisperLang(h.language));
  return text.trim();
}

async function ensureWhisperClient(h: FinishCaptureHandlers): Promise<WhisperWebClient> {
  if (!h.whisperRef.current) {
    h.whisperRef.current = new WhisperWebClient({
      onProgress: (p: WhisperWebProgress) =>
        h.setModelLoad({ state: "loading", progress: p.progress }),
    });
    h.setModelLoad({ state: "loading", progress: 0 });
  }
  try {
    await h.whisperRef.current.init(h.whisperModel);
    h.setModelLoad({ state: "ready", progress: 1 });
  } catch (err) {
    h.setModelLoad({ state: "error", progress: 0 });
    throw err;
  }
  return h.whisperRef.current;
}

// ── Hook helpers ────────────────────────────────────────────────────────

function useVoiceModePrefs() {
  return useAppStore((s) => s.userSettings.voiceMode);
}

function useCallbackRefs(opts: UseVoiceInputOptions) {
  const onTranscriptRef = useRef(opts.onTranscript);
  const onErrorRef = useRef(opts.onError);
  useEffect(() => {
    onTranscriptRef.current = opts.onTranscript;
    onErrorRef.current = opts.onError;
  });
  return { onTranscriptRef, onErrorRef };
}

// Re-init the whisper client whenever the user switches model size, so we
// don't keep an old in-memory model around when the next start() runs.
function useDisposeWhisperOnModelChange(
  whisperRef: WhisperRefBox,
  modelSize: string,
  reset: () => void,
) {
  const previousModelRef = useRef(modelSize);
  useEffect(() => {
    if (previousModelRef.current === modelSize) return;
    previousModelRef.current = modelSize;
    whisperRef.current?.dispose();
    whisperRef.current = null;
    reset();
  }, [modelSize, whisperRef, reset]);
}

function useUnmountCleanup(driverRef: DriverRefBox, whisperRef: WhisperRefBox) {
  useEffect(() => {
    return () => {
      abortDriver(driverRef);
      whisperRef.current?.dispose();
      whisperRef.current = null;
    };
  }, [driverRef, whisperRef]);
}

// ── Hook ────────────────────────────────────────────────────────────────

export function useVoiceInput(opts: UseVoiceInputOptions): UseVoiceInputResult {
  const caps = useMemo(() => detectVoiceCapabilities(), []);
  const prefs = useVoiceModePrefs();
  const enabled = opts.enabled !== false;
  const engine = useMemo(
    () => (enabled ? resolveActiveEngine(prefs.engine, caps, true) : null),
    [enabled, prefs.engine, caps],
  );
  const supported = engine !== null;

  const [state, setState] = useState<VoiceInputState>("idle");
  const [error, setError] = useState<VoiceError | null>(null);
  const [modelLoad, setModelLoad] = useState<VoiceModelLoadState>({
    state: "idle",
    progress: 0,
  });

  const driverRef = useRef<ActiveDriverRef>(null);
  const whisperRef = useRef<WhisperWebClient | null>(null);
  const { onTranscriptRef, onErrorRef } = useCallbackRefs(opts);

  const emitError = useCallback(
    (e: VoiceError) => {
      setError(e);
      setState("idle");
      onErrorRef.current?.(e);
    },
    [onErrorRef],
  );

  const resetModelLoad = useCallback(() => setModelLoad({ state: "idle", progress: 0 }), []);

  useUnmountCleanup(driverRef, whisperRef);
  useDisposeWhisperOnModelChange(whisperRef, prefs.whisperWebModel, resetModelLoad);

  const start = useCallback(async () => {
    if (!supported || !engine) {
      emitError({ code: "unsupported", message: "Voice input is not supported in this browser." });
      return;
    }
    if (state !== "idle") return;
    setError(null);
    if (engine === "webSpeech") {
      runWebSpeech({
        setState,
        driverRef,
        emitError,
        onTranscriptRef,
        lang: resolveLang(prefs.language),
      });
      return;
    }
    await beginCapture(engine, { setState, emitError, driverRef });
  }, [supported, engine, state, emitError, prefs.language, onTranscriptRef]);

  const stop = useCallback(async () => {
    const driver = driverRef.current;
    if (!driver) return;
    if (driver.kind === "webSpeech") {
      driver.recognition.stop();
      return;
    }
    await finishCapture({
      driverRef,
      whisperRef,
      setState,
      setModelLoad,
      emitError,
      onTranscriptRef,
      whisperModel: prefs.whisperWebModel,
      language: prefs.language,
    });
  }, [emitError, prefs.whisperWebModel, prefs.language, onTranscriptRef]);

  const cancel = useCallback(() => {
    abortDriver(driverRef);
    setState("idle");
    setError(null);
  }, []);

  return { supported, engine, state, error, modelLoad, start, stop, cancel };
}
