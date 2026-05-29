"use client";

import { whisperModelConfig } from "./whisper-web-models";
import type { WhisperWebModelSize } from "@/lib/types/http-voice";

/**
 * Sample rate Whisper expects. We resample the captured audio to this rate
 * (mono Float32Array) before sending to the worker — Whisper's own decoder
 * would do this too, but doing it here keeps the worker focused on inference.
 */
const WHISPER_SAMPLE_RATE = 16000;

export type WhisperWebProgress = {
  stage: string;
  progress: number;
};

export type WhisperWebHandlers = {
  onProgress?: (p: WhisperWebProgress) => void;
};

type WorkerMessage =
  | { type: "progress"; stage: string; progress: number }
  | { type: "ready" }
  | { type: "result"; text: string }
  | { type: "error"; message: string };

type Pending = {
  kind: "init" | "transcribe";
  resolve: (value: string | undefined) => void;
  reject: (err: Error) => void;
};

/**
 * Client wrapper around the whisper-web worker. Hides the postMessage
 * protocol behind a clean promise-based API and handles the audio decode +
 * resample step so callers only see "Blob in, transcript out".
 */
export class WhisperWebClient {
  private worker: Worker | null = null;
  private pending: Pending | null = null;
  private ready = false;
  private loadingModelId: string | null = null;

  constructor(private handlers: WhisperWebHandlers = {}) {}

  /**
   * Lazy-creates the worker on first use. Returns a promise that resolves
   * when the requested model is loaded and ready to transcribe.
   */
  async init(size: WhisperWebModelSize): Promise<void> {
    const config = whisperModelConfig(size);
    if (this.ready && this.loadingModelId === config.modelId) return;
    this.ensureWorker();
    this.loadingModelId = config.modelId;
    this.ready = false;
    await this.send({ kind: "init", payload: { type: "init", model: config.modelId } });
    this.ready = true;
  }

  /**
   * Transcribe a recorded blob. The blob may be in any container the browser
   * can decode (audio/webm, audio/wav, audio/mp4, …) — we resample everything
   * to 16 kHz mono Float32 before handing to the worker.
   */
  async transcribe(blob: Blob, language?: string): Promise<string> {
    if (!this.ready || !this.worker) {
      throw new Error("WhisperWebClient: not initialized");
    }
    const audio = await blobToWhisperFloat32(blob);
    const text = await this.send({
      kind: "transcribe",
      payload: { type: "transcribe", audio, language },
      transfer: [audio.buffer],
    });
    return text ?? "";
  }

  /** Tear down the worker and release the loaded model. */
  dispose(): void {
    if (this.worker) {
      try {
        this.worker.postMessage({ type: "dispose" });
      } catch {
        // ignore
      }
      this.worker.terminate();
      this.worker = null;
    }
    this.ready = false;
    this.loadingModelId = null;
    if (this.pending) {
      this.pending.reject(new Error("WhisperWebClient disposed"));
      this.pending = null;
    }
  }

  private ensureWorker() {
    if (this.worker) return;
    // The `new Worker(new URL(..., import.meta.url))` form is Next.js / webpack's
    // recommended pattern — webpack handles the bundling and asset path.
    this.worker = new Worker(new URL("../../workers/whisper-web.worker.ts", import.meta.url), {
      type: "module",
    });
    this.worker.addEventListener("message", (e: MessageEvent<WorkerMessage>) =>
      this.handleMessage(e.data),
    );
    this.worker.addEventListener("error", (e) => {
      const err = new Error(e.message || "Whisper worker crashed");
      // Tear the dead worker down so the next init()/transcribe() recreates
      // a fresh one. Without this, ensureWorker() short-circuits on the
      // still-truthy reference, posts to nothing, and the new request hangs
      // forever — bricking voice input until a full page reload.
      this.worker?.terminate();
      this.worker = null;
      this.ready = false;
      this.loadingModelId = null;
      if (this.pending) {
        this.pending.reject(err);
        this.pending = null;
      }
    });
  }

  private send(args: {
    kind: "init" | "transcribe";
    payload: object;
    transfer?: Transferable[];
  }): Promise<string | undefined> {
    if (!this.worker) throw new Error("WhisperWebClient: worker not initialized");
    if (this.pending) {
      return Promise.reject(new Error("WhisperWebClient: another request is in flight"));
    }
    return new Promise<string | undefined>((resolve, reject) => {
      this.pending = { kind: args.kind, resolve, reject };
      this.worker?.postMessage(args.payload, args.transfer ?? []);
    });
  }

  private handleMessage(msg: WorkerMessage) {
    if (msg.type === "progress") {
      this.handlers.onProgress?.({ stage: msg.stage, progress: msg.progress });
      return;
    }
    const pending = this.pending;
    if (!pending) return;
    this.pending = null;
    if (msg.type === "error") {
      pending.reject(new Error(msg.message));
      return;
    }
    if (msg.type === "ready") {
      pending.resolve(undefined);
      return;
    }
    if (msg.type === "result") {
      pending.resolve(msg.text);
    }
  }
}

/**
 * Decode an arbitrary audio Blob and return a Float32Array sampled at 16 kHz
 * mono — the format Whisper expects.
 */
export async function blobToWhisperFloat32(blob: Blob): Promise<Float32Array> {
  const arrayBuffer = await blob.arrayBuffer();
  // Decode using an AudioContext at the source rate, then bounce through an
  // OfflineAudioContext for the resample. AudioContext.decodeAudioData
  // tolerates webm/opus, mp4/aac, wav, ogg — anything the browser can play.
  const AudioCtor =
    window.AudioContext ??
    (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!AudioCtor) throw new Error("AudioContext is not available in this browser");
  const decodeCtx = new AudioCtor();
  let decoded: AudioBuffer;
  try {
    decoded = await decodeCtx.decodeAudioData(arrayBuffer);
  } finally {
    await decodeCtx.close();
  }
  return resampleToMono16k(decoded);
}

async function resampleToMono16k(buf: AudioBuffer): Promise<Float32Array> {
  const length = Math.ceil((buf.duration * WHISPER_SAMPLE_RATE) / 1);
  const offline = new OfflineAudioContext(1, length, WHISPER_SAMPLE_RATE);
  const source = offline.createBufferSource();
  source.buffer = buf;
  source.connect(offline.destination);
  source.start(0);
  const rendered = await offline.startRendering();
  return rendered.getChannelData(0).slice();
}
