/**
 * Wire types for the Voice Mode user settings. Kept in their own module so
 * http.ts stays under the 600-line file limit.
 */

export type VoiceInputEngine = "auto" | "webSpeech" | "whisperWeb" | "whisperServer";
export type VoiceInputActivationMode = "toggle" | "hold";
export type WhisperWebModelSize = "tiny" | "base" | "small";

export type VoiceModeSettings = {
  enabled: boolean;
  engine: VoiceInputEngine;
  language: string;
  mode: VoiceInputActivationMode;
  auto_send: boolean;
  whisper_web_model: WhisperWebModelSize;
};
