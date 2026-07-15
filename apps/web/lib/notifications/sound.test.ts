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
    vi.restoreAllMocks();
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

  it("does not throw when AudioContext construction fails", async () => {
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => {
        throw new Error("audio stack broken");
      }),
    );
    const { playSoundPreset } = await loadSoundModule();

    expect(() => playSoundPreset("plim")).not.toThrow();
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

describe("playSoundPreset with a non-running context", () => {
  beforeEach(() => {
    vi.resetModules();
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("plays after a suspended context resumes promptly", async () => {
    const { ctx } = makeFakeAudioContext();
    ctx.state = "suspended";
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playSoundPreset, SOUND_PRESETS } = await loadSoundModule();

    playSoundPreset("plim");
    expect(ctx.resume).toHaveBeenCalled();
    expect(ctx.createOscillator).not.toHaveBeenCalled();
    await new Promise((resolve) => setTimeout(resolve, 0));

    const plim = SOUND_PRESETS.find((p) => p.id === "plim")!;
    expect(ctx.createOscillator).toHaveBeenCalledTimes(plim.notes.length);
  });

  it("drops the play when a suspended context resumes too late", async () => {
    const { ctx } = makeFakeAudioContext();
    ctx.state = "suspended";
    let resolveResume!: () => void;
    ctx.resume = vi.fn(() => new Promise<void>((resolve) => (resolveResume = resolve)));
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(0);
    const { playSoundPreset } = await loadSoundModule();

    playSoundPreset("plim");
    nowSpy.mockReturnValue(60_000);
    resolveResume();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(ctx.createOscillator).not.toHaveBeenCalled();
  });

  it("coalesces plays queued while the context is not running", async () => {
    const { ctx } = makeFakeAudioContext();
    ctx.state = "suspended";
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playSoundPreset, SOUND_PRESETS } = await loadSoundModule();

    playSoundPreset("plim");
    playSoundPreset("plim");
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(ctx.resume).toHaveBeenCalledTimes(1);
    const plim = SOUND_PRESETS.find((p) => p.id === "plim")!;
    expect(ctx.createOscillator).toHaveBeenCalledTimes(plim.notes.length);
  });

  it("resumes an interrupted context before playing", async () => {
    const { ctx } = makeFakeAudioContext();
    ctx.state = "interrupted";
    vi.stubGlobal(
      "AudioContext",
      vi.fn(() => ctx),
    );
    const { playSoundPreset, SOUND_PRESETS } = await loadSoundModule();

    playSoundPreset("ding");
    await new Promise((resolve) => setTimeout(resolve, 0));

    const ding = SOUND_PRESETS.find((p) => p.id === "ding")!;
    expect(ctx.createOscillator).toHaveBeenCalledTimes(ding.notes.length);
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
