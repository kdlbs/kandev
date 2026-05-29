import type { WhisperWebModelSize } from "@/lib/types/http-voice";

export type WhisperModelConfig = {
  size: WhisperWebModelSize;
  /** Hugging Face model id (the Xenova/* mirrors are pre-quantized for transformers.js). */
  modelId: string;
  /** Rough on-disk size after download, shown in the settings UI. */
  approxBytes: number;
  /** Human-readable label. */
  label: string;
};

// The `onnx-community/whisper-*` mirrors are the maintained transformers.js
// exports. The older `Xenova/whisper-*` mirrors default to 4-bit (`MatMulNBits`)
// weights that only run on WebGPU — on WASM they fail with
// `Missing required scale: ... weight_merged_0_scale`. The onnx-community
// mirrors include the q8 variant we pin to in the worker.
export const WHISPER_WEB_MODELS: Record<WhisperWebModelSize, WhisperModelConfig> = {
  tiny: {
    size: "tiny",
    modelId: "onnx-community/whisper-tiny",
    approxBytes: 40 * 1024 * 1024,
    label: "Whisper Tiny",
  },
  base: {
    size: "base",
    modelId: "onnx-community/whisper-base",
    approxBytes: 75 * 1024 * 1024,
    label: "Whisper Base",
  },
  small: {
    size: "small",
    modelId: "onnx-community/whisper-small",
    approxBytes: 240 * 1024 * 1024,
    label: "Whisper Small",
  },
};

export function whisperModelConfig(size: WhisperWebModelSize): WhisperModelConfig {
  return WHISPER_WEB_MODELS[size] ?? WHISPER_WEB_MODELS.base;
}
