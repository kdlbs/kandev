"use client";

import type { VoiceInputEngine } from "@/lib/types/http-voice";

/**
 * Capability report for the voice-mode engines available in the current
 * browser. Shared between `useVoiceInput` (which picks the active engine)
 * and the Voice Mode settings page (which decides which options to render).
 */
export type VoiceCapabilities = {
  webSpeech: boolean;
  whisperWeb: boolean;
  /** True if the browser supports MediaRecorder + getUserMedia, the floor
   *  for any audio-capture engine (whisperWeb + whisperServer). */
  audioCapture: boolean;
};

/**
 * Detects which voice engines this browser can run. Safe to call during
 * SSR — returns all-false instead of throwing on missing globals.
 */
export function detectVoiceCapabilities(): VoiceCapabilities {
  if (typeof window === "undefined") {
    return { webSpeech: false, whisperWeb: false, audioCapture: false };
  }
  const w = window as Window & {
    SpeechRecognition?: unknown;
    webkitSpeechRecognition?: unknown;
  };
  const webSpeech = !!(w.SpeechRecognition || w.webkitSpeechRecognition);
  const audioCapture =
    typeof navigator !== "undefined" &&
    typeof navigator.mediaDevices?.getUserMedia === "function" &&
    typeof window.MediaRecorder !== "undefined";
  // whisper-web piggybacks on transformers.js which only needs a Worker plus
  // either WebGPU or WebAssembly. Every modern browser has both, so the
  // gating constraint is having MediaRecorder for capture.
  const whisperWeb = audioCapture && typeof Worker !== "undefined";
  return { webSpeech, whisperWeb, audioCapture };
}

/**
 * Resolves the active voice-input engine given a user preference and the
 * detected capabilities. Returns null when nothing usable is available.
 *
 * Auto-fallback order: Web Speech (cheapest, native) → Whisper Web (private,
 * heavier) → Whisper Server (always works but requires a configured server).
 * If the user pinned a specific engine that isn't available, we degrade
 * gracefully along the same order.
 */
export function resolveActiveEngine(
  preference: VoiceInputEngine,
  caps: VoiceCapabilities,
  serverFallbackEnabled: boolean,
): Exclude<VoiceInputEngine, "auto"> | null {
  const order: Array<Exclude<VoiceInputEngine, "auto">> = [
    "webSpeech",
    "whisperWeb",
    "whisperServer",
  ];

  const isUsable = (e: Exclude<VoiceInputEngine, "auto">) => {
    if (e === "webSpeech") return caps.webSpeech;
    if (e === "whisperWeb") return caps.whisperWeb;
    return caps.audioCapture && serverFallbackEnabled;
  };

  if (preference === "auto") {
    return order.find(isUsable) ?? null;
  }
  if (isUsable(preference)) return preference;
  // Pinned engine isn't usable — fall through to the next available one in
  // the auto order so the button still works instead of silently no-op.
  return order.find(isUsable) ?? null;
}
