import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

function makeFakeAudioContext() {
  const oscillator = {
    type: "",
    frequency: { value: 0 },
    connect: vi.fn(),
    start: vi.fn(),
    stop: vi.fn(),
  };
  const gain = {
    gain: {
      setValueAtTime: vi.fn(),
      linearRampToValueAtTime: vi.fn(),
      exponentialRampToValueAtTime: vi.fn(),
    },
    connect: vi.fn(),
  };
  const ctx = {
    currentTime: 0,
    state: "running",
    destination: {},
    resume: vi.fn().mockResolvedValue(undefined),
    createOscillator: vi.fn(() => oscillator),
    createGain: vi.fn(() => gain),
  };
  return { ctx, oscillator, gain };
}

async function loadSoundModule() {
  return import("./sound");
}

describe("sound preferences", () => {
  beforeEach(() => {
    vi.resetModules();
    window.localStorage.clear();
  });

  it("defaults to disabled with the plim preset", async () => {
    const { getSoundPreferences } = await loadSoundModule();
    expect(getSoundPreferences()).toEqual({ enabled: false, presetId: "plim" });
  });

  it("round-trips preferences through localStorage", async () => {
    const { getSoundPreferences, setSoundPreferences } = await loadSoundModule();
    setSoundPreferences({ enabled: true, presetId: "chime" });
    expect(getSoundPreferences()).toEqual({ enabled: true, presetId: "chime" });
  });

  it("falls back to the default preset for unknown stored ids", async () => {
    const { getSoundPreferences } = await loadSoundModule();
    window.localStorage.setItem(
      "kandev.notifications.sound",
      JSON.stringify({ enabled: true, presetId: "airhorn" }),
    );
    expect(getSoundPreferences()).toEqual({ enabled: true, presetId: "plim" });
  });

  it("ignores corrupted stored values", async () => {
    const { getSoundPreferences } = await loadSoundModule();
    window.localStorage.setItem("kandev.notifications.sound", "not-json");
    expect(getSoundPreferences()).toEqual({ enabled: false, presetId: "plim" });
  });
});

describe("playSoundPreset", () => {
  beforeEach(() => {
    vi.resetModules();
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("schedules one oscillator per note of the preset", async () => {
    const { ctx } = makeFakeAudioContext();
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playSoundPreset, SOUND_PRESETS } = await loadSoundModule();

    playSoundPreset("chime");

    const chime = SOUND_PRESETS.find((p) => p.id === "chime")!;
    expect(ctx.createOscillator).toHaveBeenCalledTimes(chime.notes.length);
  });

  it("falls back to the first preset for unknown ids", async () => {
    const { ctx } = makeFakeAudioContext();
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playSoundPreset, SOUND_PRESETS } = await loadSoundModule();

    playSoundPreset("nope");

    expect(ctx.createOscillator).toHaveBeenCalledTimes(SOUND_PRESETS[0].notes.length);
  });

  it("resumes a suspended context before playing", async () => {
    const { ctx } = makeFakeAudioContext();
    ctx.state = "suspended";
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playSoundPreset } = await loadSoundModule();

    playSoundPreset("plim");

    expect(ctx.resume).toHaveBeenCalled();
  });

  it("reuses a single AudioContext across plays", async () => {
    const { ctx } = makeFakeAudioContext();
    const ctor = vi.fn(() => ctx);
    vi.stubGlobal("AudioContext", ctor);
    const { playSoundPreset } = await loadSoundModule();

    playSoundPreset("plim");
    playSoundPreset("ding");

    expect(ctor).toHaveBeenCalledTimes(1);
  });

  it("is a no-op when AudioContext is unavailable", async () => {
    const { playSoundPreset } = await loadSoundModule();
    expect(() => playSoundPreset("plim")).not.toThrow();
  });
});

describe("playWaitingForInputSound", () => {
  beforeEach(() => {
    vi.resetModules();
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("does not play when sound is disabled (default)", async () => {
    const { ctx } = makeFakeAudioContext();
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playWaitingForInputSound } = await loadSoundModule();

    playWaitingForInputSound();

    expect(ctx.createOscillator).not.toHaveBeenCalled();
  });

  it("plays the configured preset when enabled", async () => {
    const { ctx } = makeFakeAudioContext();
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playWaitingForInputSound, setSoundPreferences, SOUND_PRESETS } =
      await loadSoundModule();
    setSoundPreferences({ enabled: true, presetId: "ding" });

    playWaitingForInputSound();

    const ding = SOUND_PRESETS.find((p) => p.id === "ding")!;
    expect(ctx.createOscillator).toHaveBeenCalledTimes(ding.notes.length);
  });
});
