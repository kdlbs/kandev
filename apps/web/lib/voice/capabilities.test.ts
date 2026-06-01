import { describe, it, expect, afterEach, vi } from "vitest";
import { detectVoiceCapabilities, resolveActiveEngine } from "./capabilities";

describe("detectVoiceCapabilities", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    delete (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition;
    delete (window as unknown as { webkitSpeechRecognition?: unknown }).webkitSpeechRecognition;
    delete (window as unknown as { MediaRecorder?: unknown }).MediaRecorder;
  });

  it("reports webSpeech true when window.SpeechRecognition exists", () => {
    (window as unknown as { SpeechRecognition: () => void }).SpeechRecognition = () => {};
    expect(detectVoiceCapabilities().webSpeech).toBe(true);
  });

  it("reports webSpeech true on the prefixed webkit variant too", () => {
    (window as unknown as { webkitSpeechRecognition: () => void }).webkitSpeechRecognition =
      () => {};
    expect(detectVoiceCapabilities().webSpeech).toBe(true);
  });

  it("reports audioCapture true when MediaRecorder + getUserMedia are present", () => {
    (window as unknown as { MediaRecorder: object }).MediaRecorder = {
      isTypeSupported: () => true,
    };
    vi.stubGlobal("navigator", { mediaDevices: { getUserMedia: () => Promise.resolve({}) } });
    expect(detectVoiceCapabilities().audioCapture).toBe(true);
  });

  it("reports everything false when no APIs are available", () => {
    vi.stubGlobal("navigator", {});
    expect(detectVoiceCapabilities()).toEqual({
      webSpeech: false,
      whisperWeb: false,
      audioCapture: false,
    });
  });
});

describe("resolveActiveEngine", () => {
  const allAvailable = { webSpeech: true, whisperWeb: true, audioCapture: true };

  it("auto picks webSpeech first when available", () => {
    expect(resolveActiveEngine("auto", allAvailable, true)).toBe("webSpeech");
  });

  it("auto falls back to whisperWeb when webSpeech is missing", () => {
    expect(
      resolveActiveEngine("auto", { webSpeech: false, whisperWeb: true, audioCapture: true }, true),
    ).toBe("whisperWeb");
  });

  it("auto falls back to whisperServer when no in-browser engine is available", () => {
    expect(
      resolveActiveEngine(
        "auto",
        { webSpeech: false, whisperWeb: false, audioCapture: true },
        true,
      ),
    ).toBe("whisperServer");
  });

  it("returns null when nothing is usable", () => {
    expect(
      resolveActiveEngine(
        "auto",
        { webSpeech: false, whisperWeb: false, audioCapture: false },
        true,
      ),
    ).toBeNull();
  });

  it("honors a pinned engine when usable", () => {
    expect(resolveActiveEngine("whisperWeb", allAvailable, true)).toBe("whisperWeb");
  });

  it("falls back along the auto order when the pinned engine is missing", () => {
    expect(
      resolveActiveEngine(
        "whisperWeb",
        { webSpeech: true, whisperWeb: false, audioCapture: true },
        true,
      ),
    ).toBe("webSpeech");
  });

  it("treats whisperServer as unusable when serverFallbackEnabled is false", () => {
    expect(
      resolveActiveEngine(
        "whisperServer",
        { webSpeech: false, whisperWeb: false, audioCapture: true },
        false,
      ),
    ).toBeNull();
  });
});
