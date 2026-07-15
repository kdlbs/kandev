import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";

// Sounds are synthesized with the Web Audio API instead of shipping binary
// audio assets: presets stay tiny, diffable, and testable in jsdom.
export type SoundNote = {
  frequency: number;
  startMs: number;
  durationMs: number;
};

export type SoundPresetId = "plim" | "chime" | "ding" | "pop";

export type SoundPreset = {
  id: SoundPresetId;
  label: string;
  notes: SoundNote[];
};

export const SOUND_PRESETS: SoundPreset[] = [
  {
    id: "plim",
    label: "Plim",
    notes: [
      { frequency: 880, startMs: 0, durationMs: 220 },
      { frequency: 1318.51, startMs: 90, durationMs: 420 },
    ],
  },
  {
    id: "chime",
    label: "Chime",
    notes: [
      { frequency: 523.25, startMs: 0, durationMs: 260 },
      { frequency: 659.25, startMs: 110, durationMs: 260 },
      { frequency: 783.99, startMs: 220, durationMs: 460 },
    ],
  },
  {
    id: "ding",
    label: "Ding",
    notes: [{ frequency: 987.77, startMs: 0, durationMs: 600 }],
  },
  {
    id: "pop",
    label: "Pop",
    notes: [{ frequency: 329.63, startMs: 0, durationMs: 140 }],
  },
];

export const DEFAULT_SOUND_PRESET_ID: SoundPresetId = "plim";

export type SoundPreferences = {
  enabled: boolean;
  presetId: SoundPresetId;
};

const SOUND_PREFS_KEY = "kandev.notifications.sound";

function isSoundPresetId(value: unknown): value is SoundPresetId {
  return SOUND_PRESETS.some((preset) => preset.id === value);
}

export function getSoundPreferences(): SoundPreferences {
  const raw = getLocalStorage<{ enabled?: boolean; presetId?: string } | null>(
    SOUND_PREFS_KEY,
    null,
  );
  return {
    enabled: raw?.enabled === true,
    presetId: isSoundPresetId(raw?.presetId) ? raw.presetId : DEFAULT_SOUND_PRESET_ID,
  };
}

export function setSoundPreferences(prefs: SoundPreferences): void {
  setLocalStorage(SOUND_PREFS_KEY, prefs);
}

const PEAK_GAIN = 0.12;
const ATTACK_SECONDS = 0.01;

let sharedContext: AudioContext | null = null;

function getSharedAudioContext(): AudioContext | null {
  if (typeof window === "undefined" || typeof window.AudioContext !== "function") return null;
  try {
    sharedContext ??= new window.AudioContext();
  } catch {
    return null;
  }
  return sharedContext;
}

function scheduleNote(ctx: AudioContext, note: SoundNote): void {
  const start = ctx.currentTime + note.startMs / 1000;
  const end = start + note.durationMs / 1000;
  const oscillator = ctx.createOscillator();
  const gain = ctx.createGain();
  oscillator.type = "sine";
  oscillator.frequency.value = note.frequency;
  gain.gain.setValueAtTime(0, start);
  gain.gain.linearRampToValueAtTime(PEAK_GAIN, start + ATTACK_SECONDS);
  gain.gain.exponentialRampToValueAtTime(0.001, end);
  oscillator.connect(gain);
  gain.connect(ctx.destination);
  oscillator.start(start);
  oscillator.stop(end);
}

// A suspended context (autoplay policy, no user gesture yet) resumes only when the
// user eventually interacts. Scheduling before resume() fulfills would queue every
// pending alert and play them stacked at that later moment — so at most one play is
// kept pending, scheduled after resume() fulfills, and dropped as stale if that
// takes longer than this.
const STALE_PLAY_TIMEOUT_MS = 2000;

let pendingResumePlay = false;

function schedulePreset(ctx: AudioContext, preset: SoundPreset): void {
  try {
    for (const note of preset.notes) scheduleNote(ctx, note);
  } catch {
    // Ignore playback failures.
  }
}

/** Best-effort playback: autoplay policy or a broken audio stack must never break the app. */
export function playSoundPreset(presetId: string): void {
  const preset = SOUND_PRESETS.find((p) => p.id === presetId) ?? SOUND_PRESETS[0];
  const ctx = getSharedAudioContext();
  if (!ctx) return;
  if (ctx.state === "running") {
    schedulePreset(ctx, preset);
    return;
  }
  // Suspended (autoplay policy) or interrupted (iOS backgrounding): resume first.
  if (pendingResumePlay) return;
  try {
    pendingResumePlay = true;
    const requestedAt = Date.now();
    ctx
      .resume()
      .then(() => {
        pendingResumePlay = false;
        if (Date.now() - requestedAt < STALE_PLAY_TIMEOUT_MS) schedulePreset(ctx, preset);
      })
      .catch(() => {
        pendingResumePlay = false;
      });
  } catch {
    pendingResumePlay = false;
  }
}

/** Play the configured sound if the user opted in on this device. */
export function playWaitingForInputSound(): void {
  const prefs = getSoundPreferences();
  if (!prefs.enabled) return;
  playSoundPreset(prefs.presetId);
}
