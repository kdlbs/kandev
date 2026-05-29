/// <reference lib="webworker" />

/**
 * Web Worker that runs OpenAI Whisper entirely in the browser via
 * @huggingface/transformers (the maintained transformers.js library that
 * xenova/whisper-web is built on).
 *
 * Lives in its own worker because model loading + inference both block the
 * main thread for several seconds — would freeze the chat input otherwise.
 *
 * Wire protocol (postMessage):
 *   in:  { type: "init",       model: "Xenova/whisper-base" }
 *   in:  { type: "transcribe", audio: Float32Array, language?: string }
 *   in:  { type: "dispose" }
 *   out: { type: "progress",   stage: string, progress: number }
 *   out: { type: "ready" }
 *   out: { type: "result",     text: string }
 *   out: { type: "error",      message: string }
 */

import { pipeline, env, type AutomaticSpeechRecognitionPipeline } from "@huggingface/transformers";

// Disable transformers.js's local-models lookup — we only load from the HF
// CDN so the worker doesn't try to fetch files from our own origin.
env.allowLocalModels = false;
env.allowRemoteModels = true;

type InitMessage = { type: "init"; model: string };
type TranscribeMessage = { type: "transcribe"; audio: Float32Array; language?: string };
type DisposeMessage = { type: "dispose" };
type InMessage = InitMessage | TranscribeMessage | DisposeMessage;

type OutMessage =
  | { type: "progress"; stage: string; progress: number }
  | { type: "ready" }
  | { type: "result"; text: string }
  | { type: "error"; message: string };

const ctx = self as unknown as DedicatedWorkerGlobalScope;

let asrPipeline: AutomaticSpeechRecognitionPipeline | null = null;
let activeModelId: string | null = null;

function post(message: OutMessage) {
  ctx.postMessage(message);
}

type ProgressEvent = {
  status?: string;
  file?: string;
  progress?: number;
};

async function handleInit(msg: InitMessage) {
  if (asrPipeline && activeModelId === msg.model) {
    post({ type: "ready" });
    return;
  }
  if (asrPipeline) {
    await asrPipeline.dispose();
    asrPipeline = null;
  }
  try {
    // dtype choice rationale: the `_quantized` / `q8` and `q4` decoder weights
    // for whisper-base both contain `MatMulNBits` ops that only execute on
    // WebGPU. On browsers without WebGPU (most Firefox, older Chrome) onnxruntime
    // throws `Missing required scale: ... weight_merged_0_scale`. fp16 has no
    // quantized ops at all so it works on both WASM and WebGPU; it's ~half the
    // size of fp32 with no perceptible accuracy loss for ASR.
    const created = await pipeline("automatic-speech-recognition", msg.model, {
      dtype: {
        encoder_model: "fp32",
        decoder_model_merged: "fp16",
      },
      progress_callback: (e: ProgressEvent) => {
        if (typeof e?.progress === "number") {
          post({
            type: "progress",
            stage: e.status ?? "download",
            progress: e.progress,
          });
        }
      },
    });
    asrPipeline = created as AutomaticSpeechRecognitionPipeline;
    activeModelId = msg.model;
    post({ type: "ready" });
  } catch (err) {
    post({ type: "error", message: errorMessage(err) });
  }
}

async function handleTranscribe(msg: TranscribeMessage) {
  if (!asrPipeline) {
    post({ type: "error", message: "Whisper worker not initialized" });
    return;
  }
  try {
    const result = (await asrPipeline(msg.audio, {
      language: msg.language && msg.language !== "auto" ? msg.language : undefined,
      task: "transcribe",
    })) as { text?: string } | Array<{ text?: string }>;
    const text = Array.isArray(result)
      ? result.map((r) => r.text ?? "").join(" ")
      : (result.text ?? "");
    post({ type: "result", text: text.trim() });
  } catch (err) {
    post({ type: "error", message: errorMessage(err) });
  }
}

async function handleDispose() {
  if (asrPipeline) {
    await asrPipeline.dispose();
    asrPipeline = null;
    activeModelId = null;
  }
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err);
}

ctx.addEventListener("message", (event: MessageEvent<InMessage>) => {
  const msg = event.data;
  switch (msg.type) {
    case "init":
      void handleInit(msg);
      break;
    case "transcribe":
      void handleTranscribe(msg);
      break;
    case "dispose":
      void handleDispose();
      break;
  }
});
