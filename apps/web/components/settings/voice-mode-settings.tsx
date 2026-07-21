"use client";

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { IconAlertTriangle, IconMicrophone } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsSection } from "@/components/settings/settings-section";
import { ShortcutRecorder } from "@/components/settings/keyboard-shortcuts-card";
import { detectVoiceCapabilities, type VoiceCapabilities } from "@/lib/voice/capabilities";
import type { UserSettingsState, VoiceModeState } from "@/lib/state/slices/settings/types";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import {
  CONFIGURABLE_SHORTCUTS,
  getShortcut,
  type StoredShortcutOverrides,
} from "@/lib/keyboard/shortcut-overrides";
import type {
  VoiceInputActivationMode,
  VoiceInputEngine,
  VoiceModeSettings as VoiceModeWire,
  WhisperWebModelSize,
} from "@/lib/types/http-voice";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { SettingsCard } from "./settings-card";

// Single source of truth for the language options. Web Speech reads `lang`,
// Whisper engines treat it as a hint. "auto" defers to the browser locale.
const LANGUAGE_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "auto", label: "Auto-detect (browser language)" },
  { value: "en-US", label: "English (United States)" },
  { value: "en-GB", label: "English (United Kingdom)" },
  { value: "es-ES", label: "Spanish (Spain)" },
  { value: "es-MX", label: "Spanish (Mexico)" },
  { value: "pt-PT", label: "Portuguese (Portugal)" },
  { value: "pt-BR", label: "Portuguese (Brazil)" },
  { value: "fr-FR", label: "French" },
  { value: "de-DE", label: "German" },
  { value: "it-IT", label: "Italian" },
  { value: "ja-JP", label: "Japanese" },
  { value: "zh-CN", label: "Chinese (Simplified)" },
];

const WHISPER_MODELS: Array<{
  value: WhisperWebModelSize;
  label: string;
  size: string;
  hint: string;
}> = [
  { value: "tiny", label: "Tiny", size: "~40 MB", hint: "Fastest, lower accuracy" },
  { value: "base", label: "Base", size: "~75 MB", hint: "Balanced default" },
  { value: "small", label: "Small", size: "~240 MB", hint: "Best accuracy, slower load" },
];

function toWire(state: VoiceModeState): VoiceModeWire {
  return {
    enabled: state.enabled,
    engine: state.engine,
    language: state.language,
    mode: state.mode,
    auto_send: state.autoSend,
    whisper_web_model: state.whisperWebModel,
  };
}

type VoiceDraft = {
  voiceMode: VoiceModeState;
  keyboardShortcuts: StoredShortcutOverrides;
};

type VoiceDraftContextValue = VoiceDraft & {
  savedVoiceMode: VoiceModeState;
  savedKeyboardShortcuts: StoredShortcutOverrides;
  updateVoiceMode: (patch: Partial<VoiceModeState>) => void;
  updateShortcuts: (shortcuts: StoredShortcutOverrides) => void;
};

const VoiceDraftContext = createContext<VoiceDraftContextValue | null>(null);

function voiceDraftFromSettings(settings: UserSettingsState): VoiceDraft {
  return {
    voiceMode: settings.voiceMode,
    keyboardShortcuts: settings.keyboardShortcuts,
  };
}

function useVoiceDraft() {
  const value = useContext(VoiceDraftContext);
  if (!value) throw new Error("Voice settings require VoiceDraftProvider");
  return value;
}

function VoiceDraftProvider({ children }: { children: ReactNode }) {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const currentSettingsDraft = voiceDraftFromSettings(userSettings);
  const [saved, setSaved] = useState<VoiceDraft>(currentSettingsDraft);
  const [draft, setDraft] = useState(saved);
  const revision = JSON.stringify(draft);
  const savedRevision = JSON.stringify(saved);
  const currentSettingsRevision = JSON.stringify(currentSettingsDraft);

  if (revision === savedRevision && currentSettingsRevision !== savedRevision) {
    setSaved(currentSettingsDraft);
    setDraft(currentSettingsDraft);
  }

  useSettingsSaveContributor({
    id: "voice-mode",
    revision,
    isDirty: revision !== savedRevision,
    save: async () => {
      const latest = voiceDraftFromSettings(storeApi.getState().userSettings);
      const submitted = {
        voiceMode:
          JSON.stringify(draft.voiceMode) === JSON.stringify(saved.voiceMode)
            ? latest.voiceMode
            : draft.voiceMode,
        keyboardShortcuts:
          JSON.stringify(draft.keyboardShortcuts) === JSON.stringify(saved.keyboardShortcuts)
            ? latest.keyboardShortcuts
            : draft.keyboardShortcuts,
      };
      await updateUserSettings({
        voice_mode: toWire(submitted.voiceMode),
        keyboard_shortcuts: submitted.keyboardShortcuts,
      });
      setSaved(submitted);
      setDraft((current) => ({
        voiceMode:
          JSON.stringify(current.voiceMode) === JSON.stringify(draft.voiceMode)
            ? submitted.voiceMode
            : current.voiceMode,
        keyboardShortcuts:
          JSON.stringify(current.keyboardShortcuts) === JSON.stringify(draft.keyboardShortcuts)
            ? submitted.keyboardShortcuts
            : current.keyboardShortcuts,
      }));
      setUserSettings({ ...storeApi.getState().userSettings, ...submitted });
    },
    discard: () => setDraft(saved),
  });

  const value = useMemo<VoiceDraftContextValue>(
    () => ({
      ...draft,
      savedVoiceMode: saved.voiceMode,
      savedKeyboardShortcuts: saved.keyboardShortcuts,
      updateVoiceMode: (patch) =>
        setDraft((current) => ({
          ...current,
          voiceMode: { ...current.voiceMode, ...patch },
        })),
      updateShortcuts: (keyboardShortcuts) =>
        setDraft((current) => ({ ...current, keyboardShortcuts })),
    }),
    [draft, saved],
  );

  return <VoiceDraftContext.Provider value={value}>{children}</VoiceDraftContext.Provider>;
}

// ── Draft hook ───────────────────────────────────────────────────────────

function useVoiceModeSaver() {
  const { updateVoiceMode } = useVoiceDraft();
  return { save: updateVoiceMode, saving: false };
}

// ── Engine card ──────────────────────────────────────────────────────────

type EngineOption = {
  value: VoiceInputEngine;
  label: string;
  description: string;
  badge?: string;
  disabled?: boolean;
};

function buildEngineOptions(caps: VoiceCapabilities): EngineOption[] {
  return [
    {
      value: "auto",
      label: "Automatic",
      description: "Use the best engine available in this browser.",
    },
    {
      value: "webSpeech",
      label: "Web Speech (in-browser)",
      description: caps.webSpeech
        ? "Free, instant, uses your browser's built-in speech recognition."
        : "Not supported in this browser.",
      disabled: !caps.webSpeech,
    },
    {
      value: "whisperWeb",
      label: "Whisper Web (private, in-browser)",
      description: caps.whisperWeb
        ? "Runs OpenAI Whisper entirely on this device. First use downloads the model (40–240 MB)."
        : "Not supported in this browser.",
      badge: "Local",
      disabled: !caps.whisperWeb,
    },
    {
      value: "whisperServer",
      label: "Whisper Server (OpenAI)",
      description: caps.audioCapture
        ? "Sends audio to the backend, which forwards it to OpenAI's Whisper API. Requires a configured API key on the server."
        : "Not supported in this browser.",
      badge: "Server",
      disabled: !caps.audioCapture,
    },
  ];
}

function EngineCard({ caps }: { caps: VoiceCapabilities }) {
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  const options = useMemo(() => buildEngineOptions(caps), [caps]);

  return (
    <SettingsCard isDirty={voiceMode.engine !== savedVoiceMode.engine}>
      <CardHeader>
        <CardTitle className="text-base">Transcription Engine</CardTitle>
      </CardHeader>
      <CardContent>
        <RadioGroup
          value={voiceMode.engine}
          onValueChange={(v) => save({ engine: v as VoiceInputEngine })}
          disabled={saving}
          className="space-y-3"
        >
          {options.map((opt) => (
            <Label
              key={opt.value}
              htmlFor={`voice-engine-${opt.value}`}
              className={`flex items-start gap-3 rounded-md border p-3 ${
                opt.disabled ? "opacity-50" : "cursor-pointer hover:bg-muted/30"
              }`}
              data-settings-dirty={
                voiceMode.engine !== savedVoiceMode.engine && opt.value === voiceMode.engine
              }
            >
              <RadioGroupItem
                id={`voice-engine-${opt.value}`}
                value={opt.value}
                disabled={opt.disabled}
                className="mt-0.5"
              />
              <div className="space-y-1">
                <div className="flex items-center gap-2 text-sm font-medium">
                  {opt.label}
                  {opt.badge && <Badge variant="secondary">{opt.badge}</Badge>}
                </div>
                <p className="text-xs text-muted-foreground">{opt.description}</p>
              </div>
            </Label>
          ))}
        </RadioGroup>
      </CardContent>
    </SettingsCard>
  );
}

// ── Behavior card (language + mode + auto-send) ──────────────────────────

function LanguageRow() {
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="space-y-2">
      <Label htmlFor="voice-language">Language</Label>
      <Select
        value={voiceMode.language}
        onValueChange={(v) => save({ language: v })}
        disabled={saving}
      >
        <SelectTrigger
          id="voice-language"
          data-settings-dirty={voiceMode.language !== savedVoiceMode.language}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectLabel>Languages</SelectLabel>
            {LANGUAGE_OPTIONS.map((l) => (
              <SelectItem key={l.value} value={l.value}>
                {l.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">
        Recognition quality drops sharply when the language doesn&apos;t match what you&apos;re
        speaking.
      </p>
    </div>
  );
}

function ModeRow() {
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="space-y-2">
      <Label>Activation</Label>
      <RadioGroup
        value={voiceMode.mode}
        onValueChange={(v) => save({ mode: v as VoiceInputActivationMode })}
        disabled={saving}
        className="flex gap-4"
        data-settings-dirty={voiceMode.mode !== savedVoiceMode.mode}
      >
        <Label htmlFor="voice-mode-toggle" className="flex items-center gap-2 cursor-pointer">
          <RadioGroupItem id="voice-mode-toggle" value="toggle" />
          <span className="text-sm">Click to start / stop</span>
        </Label>
        <Label htmlFor="voice-mode-hold" className="flex items-center gap-2 cursor-pointer">
          <RadioGroupItem id="voice-mode-hold" value="hold" />
          <span className="text-sm">Hold to talk</span>
        </Label>
      </RadioGroup>
    </div>
  );
}

function AutoSendRow() {
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="flex items-center justify-between">
      <div className="space-y-1">
        <Label htmlFor="voice-auto-send" className="cursor-pointer">
          Auto-send after transcription
        </Label>
        <p className="text-xs text-muted-foreground">
          Submit the message as soon as the transcript is inserted.
        </p>
      </div>
      <Switch
        id="voice-auto-send"
        checked={voiceMode.autoSend}
        onCheckedChange={(checked) => save({ autoSend: checked })}
        disabled={saving}
        data-settings-dirty={voiceMode.autoSend !== savedVoiceMode.autoSend}
      />
    </div>
  );
}

function BehaviorCard() {
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const isDirty =
    voiceMode.language !== savedVoiceMode.language ||
    voiceMode.mode !== savedVoiceMode.mode ||
    voiceMode.autoSend !== savedVoiceMode.autoSend;
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle className="text-base">Behavior</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <LanguageRow />
        <ModeRow />
        <AutoSendRow />
      </CardContent>
    </SettingsCard>
  );
}

// ── Whisper Web model card ───────────────────────────────────────────────

function WhisperModelCard() {
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();

  return (
    <SettingsCard isDirty={voiceMode.whisperWebModel !== savedVoiceMode.whisperWebModel}>
      <CardHeader>
        <CardTitle className="text-base">Whisper Web Model</CardTitle>
      </CardHeader>
      <CardContent>
        <RadioGroup
          value={voiceMode.whisperWebModel}
          onValueChange={(v) => save({ whisperWebModel: v as WhisperWebModelSize })}
          disabled={saving}
          className="space-y-2"
        >
          {WHISPER_MODELS.map((m) => (
            <Label
              key={m.value}
              htmlFor={`whisper-model-${m.value}`}
              className="flex items-start gap-3 rounded-md border p-3 cursor-pointer hover:bg-muted/30"
              data-settings-dirty={
                voiceMode.whisperWebModel !== savedVoiceMode.whisperWebModel &&
                m.value === voiceMode.whisperWebModel
              }
            >
              <RadioGroupItem id={`whisper-model-${m.value}`} value={m.value} className="mt-0.5" />
              <div>
                <div className="text-sm font-medium">
                  {m.label} <span className="text-muted-foreground font-normal">· {m.size}</span>
                </div>
                <p className="text-xs text-muted-foreground">{m.hint}</p>
              </div>
            </Label>
          ))}
        </RadioGroup>
        <p className="text-xs text-muted-foreground mt-3">
          The model downloads on first use and is cached in your browser. Switching models triggers
          another download next time you record.
        </p>
      </CardContent>
    </SettingsCard>
  );
}

// ── Enable card (top-level on/off) ───────────────────────────────────────

function EnableCard() {
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <SettingsCard
      isDirty={voiceMode.enabled !== savedVoiceMode.enabled}
      data-testid="voice-enable-card"
    >
      <CardHeader>
        <CardTitle className="text-base">Enable Voice Input</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <Label htmlFor="voice-enabled" className="cursor-pointer">
              Show the mic button on the chat composer
            </Label>
            <p className="text-xs text-muted-foreground">
              When off, the voice button is hidden entirely and no voice-related code runs. Settings
              below are preserved and re-applied when you turn it back on.
            </p>
          </div>
          <Switch
            id="voice-enabled"
            checked={voiceMode.enabled}
            onCheckedChange={(checked) => save({ enabled: checked })}
            disabled={saving}
            data-settings-dirty={voiceMode.enabled !== savedVoiceMode.enabled}
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}

// ── Availability banner ──────────────────────────────────────────────────

function AvailabilityBanner({ caps }: { caps: VoiceCapabilities }) {
  if (caps.webSpeech || caps.whisperWeb || caps.audioCapture) return null;
  // Secure-context requirement is the most common reason capability detection
  // returns all-false on mobile (when reaching the dev server over LAN HTTP).
  // Spell it out so the user doesn't have to guess.
  const insecure = typeof window !== "undefined" && !window.isSecureContext;
  return (
    <div className="flex items-start gap-3 rounded-md border border-orange-500/40 bg-orange-500/5 p-3">
      <IconAlertTriangle className="h-5 w-5 text-orange-500 shrink-0 mt-0.5" />
      <div className="space-y-1 text-sm">
        <p className="font-medium">Voice input is unavailable in this browser.</p>
        <p className="text-xs text-muted-foreground">
          {insecure
            ? "Microphone APIs require HTTPS or localhost. You appear to be on an insecure HTTP origin — load this page over HTTPS (or http://localhost) to enable voice input."
            : "Your browser doesn't expose either the Web Speech API or MediaRecorder. Try Chrome, Edge, or Safari 14.5+."}
        </p>
      </div>
    </div>
  );
}

// ── Voice keyboard shortcut card ─────────────────────────────────────────

function useShortcutSaver() {
  return useVoiceDraft().updateShortcuts;
}

function VoiceShortcutCard() {
  const { keyboardShortcuts: overrides, savedKeyboardShortcuts } = useVoiceDraft();
  const persist = useShortcutSaver();
  const current = getShortcut("VOICE_INPUT_TOGGLE", overrides);
  const savedCurrent = getShortcut("VOICE_INPUT_TOGGLE", savedKeyboardShortcuts);
  const isDirty = JSON.stringify(current) !== JSON.stringify(savedCurrent);

  const handleChange = useCallback(
    (_id: string, shortcut: KeyboardShortcut) =>
      persist({ ...overrides, VOICE_INPUT_TOGGLE: shortcut }),
    [overrides, persist],
  );
  const handleReset = useCallback(() => {
    const next = { ...overrides };
    delete next.VOICE_INPUT_TOGGLE;
    persist(next);
  }, [overrides, persist]);

  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle className="text-base">
          {CONFIGURABLE_SHORTCUTS.VOICE_INPUT_TOGGLE.label} Shortcut
        </CardTitle>
      </CardHeader>
      <CardContent>
        <ShortcutRecorder
          shortcutId="VOICE_INPUT_TOGGLE"
          current={current}
          onChange={handleChange}
          onReset={handleReset}
          isDirty={isDirty}
        />
        <p className="text-xs text-muted-foreground mt-2">
          Click the shortcut to record a new key combination. All keyboard shortcuts can also be
          edited in General Settings.
        </p>
      </CardContent>
    </SettingsCard>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────

function VoiceModeSettingsContent() {
  const caps = useMemo(() => detectVoiceCapabilities(), []);
  const { voiceMode } = useVoiceDraft();
  const enabled = voiceMode.enabled;
  return (
    <SettingsSection
      icon={<IconMicrophone className="h-5 w-5" />}
      title="Voice Mode"
      description="Configure how voice input works on the chat composer."
    >
      <div className="space-y-4">
        <EnableCard />
        {/* When voice is disabled, keep showing the secondary cards but dim
            them — preserves the visible configuration without implying it has
            any effect right now. */}
        <div className={enabled ? undefined : "opacity-50 pointer-events-none"}>
          <div className="space-y-4">
            <AvailabilityBanner caps={caps} />
            <EngineCard caps={caps} />
            <BehaviorCard />
            <WhisperModelCard />
            <VoiceShortcutCard />
          </div>
        </div>
      </div>
    </SettingsSection>
  );
}

export function VoiceModeSettings() {
  return (
    <VoiceDraftProvider>
      <VoiceModeSettingsContent />
    </VoiceDraftProvider>
  );
}
