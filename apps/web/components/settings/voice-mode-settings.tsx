"use client";

import { useCallback, useMemo, useState } from "react";
import { IconAlertTriangle, IconMicrophone } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
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
import { useToast } from "@/components/toast-provider";
import { updateUserSettings } from "@/lib/api";
import { SettingsSection } from "@/components/settings/settings-section";
import { ShortcutRecorder } from "@/components/settings/keyboard-shortcuts-card";
import { detectVoiceCapabilities, type VoiceCapabilities } from "@/lib/voice/capabilities";
import type { VoiceModeState } from "@/lib/state/slices/settings/types";
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

// ── Save hook ────────────────────────────────────────────────────────────

function useVoiceModeSaver() {
  // Read userSettings via the store API (not as a React selector) so the
  // async save handler reads the latest snapshot at invocation time instead
  // of capturing a stale closure. Without this, concurrent settings updates
  // racing with this save (or a rejection rolling back to a stale snapshot)
  // can silently overwrite unrelated fields.
  const storeApi = useAppStoreApi();
  const setUserSettings = useAppStore((s) => s.setUserSettings);
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);

  const save = useCallback(
    async (patch: Partial<VoiceModeState>) => {
      const current = storeApi.getState().userSettings;
      const previous = current.voiceMode;
      const next = { ...previous, ...patch };
      setUserSettings({ ...current, voiceMode: next });
      setSaving(true);
      try {
        await updateUserSettings({ voice_mode: toWire(next) });
      } catch {
        // Rollback only the keys this request changed AND only when the live
        // value still matches what we optimistically wrote. If a newer save
        // for the same key landed first, that's now the truth — reverting
        // would silently roll back the user's later edit.
        const latest = storeApi.getState().userSettings;
        const reverted: Partial<VoiceModeState> = {};
        for (const key of Object.keys(patch) as Array<keyof VoiceModeState>) {
          if (latest.voiceMode[key] !== next[key]) continue;
          // Cast through unknown so the per-key assignment passes strict checks.
          (reverted as Record<string, unknown>)[key] = previous[key];
        }
        setUserSettings({
          ...latest,
          voiceMode: { ...latest.voiceMode, ...reverted },
        });
        toast({ title: "Failed to save Voice Mode setting", variant: "error" });
      } finally {
        setSaving(false);
      }
    },
    [storeApi, setUserSettings, toast],
  );

  return { save, saving };
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
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
  const { save, saving } = useVoiceModeSaver();
  const options = useMemo(() => buildEngineOptions(caps), [caps]);

  return (
    <Card>
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
    </Card>
  );
}

// ── Behavior card (language + mode + auto-send) ──────────────────────────

function LanguageRow() {
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="space-y-2">
      <Label htmlFor="voice-language">Language</Label>
      <Select
        value={voiceMode.language}
        onValueChange={(v) => save({ language: v })}
        disabled={saving}
      >
        <SelectTrigger id="voice-language">
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
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="space-y-2">
      <Label>Activation</Label>
      <RadioGroup
        value={voiceMode.mode}
        onValueChange={(v) => save({ mode: v as VoiceInputActivationMode })}
        disabled={saving}
        className="flex gap-4"
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
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
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
      />
    </div>
  );
}

function BehaviorCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Behavior</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <LanguageRow />
        <ModeRow />
        <AutoSendRow />
      </CardContent>
    </Card>
  );
}

// ── Whisper Web model card ───────────────────────────────────────────────

function WhisperModelCard() {
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
  const { save, saving } = useVoiceModeSaver();

  return (
    <Card>
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
    </Card>
  );
}

// ── Enable card (top-level on/off) ───────────────────────────────────────

function EnableCard() {
  const voiceMode = useAppStore((s) => s.userSettings.voiceMode);
  const { save, saving } = useVoiceModeSaver();
  return (
    <Card>
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
          />
        </div>
      </CardContent>
    </Card>
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
  // Same stale-closure protection as useVoiceModeSaver — read live store
  // state at call time so a concurrent keyboard-shortcut change from another
  // settings card isn't clobbered by this card's optimistic update / rollback.
  const storeApi = useAppStoreApi();
  const setUserSettings = useAppStore((s) => s.setUserSettings);
  const { toast } = useToast();
  return useCallback(
    (next: StoredShortcutOverrides) => {
      const current = storeApi.getState().userSettings;
      const previous = current.keyboardShortcuts;
      setUserSettings({ ...current, keyboardShortcuts: next });
      updateUserSettings({ keyboard_shortcuts: next }).catch(() => {
        // Rollback only the keys this request changed AND only when the live
        // value still matches what we optimistically wrote. Skip otherwise so
        // a newer successful save to the same key isn't reverted.
        const latest = storeApi.getState().userSettings;
        const restored: StoredShortcutOverrides = { ...latest.keyboardShortcuts };
        const changedKeys = new Set([...Object.keys(previous), ...Object.keys(next)]);
        for (const key of changedKeys) {
          if (previous[key] === next[key]) continue;
          if (latest.keyboardShortcuts[key] !== next[key]) continue;
          if (previous[key] === undefined) delete restored[key];
          else restored[key] = previous[key];
        }
        setUserSettings({ ...latest, keyboardShortcuts: restored });
        toast({ title: "Failed to save shortcut", variant: "error" });
      });
    },
    [storeApi, setUserSettings, toast],
  );
}

function VoiceShortcutCard() {
  const overrides = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const persist = useShortcutSaver();
  const current = getShortcut("VOICE_INPUT_TOGGLE", overrides);

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
    <Card>
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
        />
        <p className="text-xs text-muted-foreground mt-2">
          Click the shortcut to record a new key combination. All keyboard shortcuts can also be
          edited in General Settings.
        </p>
      </CardContent>
    </Card>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────

export function VoiceModeSettings() {
  const caps = useMemo(() => detectVoiceCapabilities(), []);
  const enabled = useAppStore((s) => s.userSettings.voiceMode.enabled);
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
