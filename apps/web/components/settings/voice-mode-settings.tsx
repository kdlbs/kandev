"use client";

import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
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
// `value` is the BCP-47 tag handed to the engine, so it is never translated;
// the labels travel as catalog keys and resolve at render.
const LANGUAGE_OPTIONS: Array<{ value: string; labelKey: string }> = [
  { value: "auto", labelKey: "settings:voiceLanguageAuto" },
  { value: "en-US", labelKey: "settings:voiceLanguageEnUs" },
  { value: "en-GB", labelKey: "settings:voiceLanguageEnGb" },
  { value: "es-ES", labelKey: "settings:voiceLanguageEsEs" },
  { value: "es-MX", labelKey: "settings:voiceLanguageEsMx" },
  { value: "pt-PT", labelKey: "settings:voiceLanguagePtPt" },
  { value: "pt-BR", labelKey: "settings:voiceLanguagePtBr" },
  { value: "fr-FR", labelKey: "settings:voiceLanguageFrFr" },
  { value: "de-DE", labelKey: "settings:voiceLanguageDeDe" },
  { value: "it-IT", labelKey: "settings:voiceLanguageItIt" },
  { value: "ja-JP", labelKey: "settings:voiceLanguageJaJp" },
  { value: "zh-CN", labelKey: "settings:voiceLanguageZhCn" },
];

// `value` is the persisted `WhisperWebModelSize` enum and `size` is a download
// size in binary units, neither of which is prose. Label and hint are copy.
const WHISPER_MODELS: Array<{
  value: WhisperWebModelSize;
  labelKey: string;
  size: string;
  hintKey: string;
}> = [
  {
    value: "tiny",
    labelKey: "settings:voiceWhisperTiny",
    size: "~40 MB",
    hintKey: "settings:voiceWhisperTinyHint",
  },
  {
    value: "base",
    labelKey: "settings:voiceWhisperBase",
    size: "~75 MB",
    hintKey: "settings:voiceWhisperBaseHint",
  },
  {
    value: "small",
    labelKey: "settings:voiceWhisperSmall",
    size: "~240 MB",
    hintKey: "settings:voiceWhisperSmallHint",
  },
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

// `value` is the persisted `VoiceInputEngine` enum; everything else is copy and
// travels as a catalog key, because this builder holds no JSX and is therefore
// invisible to the literal guard.
type EngineOption = {
  value: VoiceInputEngine;
  labelKey: string;
  descriptionKey: string;
  badgeKey?: string;
  disabled?: boolean;
};

const ENGINE_UNSUPPORTED_KEY = "settings:voiceEngineUnsupported";

function buildEngineOptions(caps: VoiceCapabilities): EngineOption[] {
  return [
    {
      value: "auto",
      labelKey: "settings:voiceEngineAuto",
      descriptionKey: "settings:voiceEngineAutoDescription",
    },
    {
      value: "webSpeech",
      labelKey: "settings:voiceEngineWebSpeech",
      descriptionKey: caps.webSpeech
        ? "settings:voiceEngineWebSpeechDescription"
        : ENGINE_UNSUPPORTED_KEY,
      disabled: !caps.webSpeech,
    },
    {
      value: "whisperWeb",
      labelKey: "settings:voiceEngineWhisperWeb",
      descriptionKey: caps.whisperWeb
        ? "settings:voiceEngineWhisperWebDescription"
        : ENGINE_UNSUPPORTED_KEY,
      badgeKey: "settings:voiceEngineBadgeLocal",
      disabled: !caps.whisperWeb,
    },
    {
      value: "whisperServer",
      labelKey: "settings:voiceEngineWhisperServer",
      descriptionKey: caps.audioCapture
        ? "settings:voiceEngineWhisperServerDescription"
        : ENGINE_UNSUPPORTED_KEY,
      badgeKey: "settings:voiceEngineBadgeServer",
      disabled: !caps.audioCapture,
    },
  ];
}

function EngineCard({ caps }: { caps: VoiceCapabilities }) {
  const { t } = useTranslation();
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  const options = useMemo(() => buildEngineOptions(caps), [caps]);

  return (
    <SettingsCard isDirty={voiceMode.engine !== savedVoiceMode.engine}>
      <CardHeader>
        <CardTitle className="text-base">{t("settings:voiceEngineTitle")}</CardTitle>
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
                  {t(opt.labelKey)}
                  {opt.badgeKey && <Badge variant="secondary">{t(opt.badgeKey)}</Badge>}
                </div>
                <p className="text-xs text-muted-foreground">{t(opt.descriptionKey)}</p>
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
  const { t } = useTranslation();
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="space-y-2">
      <Label htmlFor="voice-language">{t("settings:language")}</Label>
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
            <SelectLabel>{t("settings:voiceLanguagesGroup")}</SelectLabel>
            {LANGUAGE_OPTIONS.map((l) => (
              <SelectItem key={l.value} value={l.value}>
                {t(l.labelKey)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("settings:voiceLanguageHint")}</p>
    </div>
  );
}

function ModeRow() {
  const { t } = useTranslation();
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="space-y-2">
      <Label>{t("settings:voiceActivation")}</Label>
      <RadioGroup
        value={voiceMode.mode}
        onValueChange={(v) => save({ mode: v as VoiceInputActivationMode })}
        disabled={saving}
        className="flex gap-4"
        data-settings-dirty={voiceMode.mode !== savedVoiceMode.mode}
      >
        <Label htmlFor="voice-mode-toggle" className="flex items-center gap-2 cursor-pointer">
          <RadioGroupItem id="voice-mode-toggle" value="toggle" />
          <span className="text-sm">{t("settings:voiceActivationToggle")}</span>
        </Label>
        <Label htmlFor="voice-mode-hold" className="flex items-center gap-2 cursor-pointer">
          <RadioGroupItem id="voice-mode-hold" value="hold" />
          <span className="text-sm">{t("settings:voiceActivationHold")}</span>
        </Label>
      </RadioGroup>
    </div>
  );
}

function AutoSendRow() {
  const { t } = useTranslation();
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <div className="flex items-center justify-between">
      <div className="space-y-1">
        <Label htmlFor="voice-auto-send" className="cursor-pointer">
          {t("settings:voiceAutoSend")}
        </Label>
        <p className="text-xs text-muted-foreground">{t("settings:voiceAutoSendDescription")}</p>
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
  const { t } = useTranslation();
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const isDirty =
    voiceMode.language !== savedVoiceMode.language ||
    voiceMode.mode !== savedVoiceMode.mode ||
    voiceMode.autoSend !== savedVoiceMode.autoSend;
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle className="text-base">{t("settings:voiceBehavior")}</CardTitle>
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
  const { t } = useTranslation();
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();

  return (
    <SettingsCard isDirty={voiceMode.whisperWebModel !== savedVoiceMode.whisperWebModel}>
      <CardHeader>
        <CardTitle className="text-base">{t("settings:voiceWhisperModelTitle")}</CardTitle>
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
                  {t(m.labelKey)}{" "}
                  <span className="text-muted-foreground font-normal">· {m.size}</span>
                </div>
                <p className="text-xs text-muted-foreground">{t(m.hintKey)}</p>
              </div>
            </Label>
          ))}
        </RadioGroup>
        <p className="text-xs text-muted-foreground mt-3">{t("settings:voiceWhisperModelHint")}</p>
      </CardContent>
    </SettingsCard>
  );
}

// ── Enable card (top-level on/off) ───────────────────────────────────────

function EnableCard() {
  const { t } = useTranslation();
  const { voiceMode, savedVoiceMode } = useVoiceDraft();
  const { save, saving } = useVoiceModeSaver();
  return (
    <SettingsCard
      isDirty={voiceMode.enabled !== savedVoiceMode.enabled}
      data-testid="voice-enable-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:voiceEnableTitle")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <Label htmlFor="voice-enabled" className="cursor-pointer">
              {t("settings:voiceEnableLabel")}
            </Label>
            <p className="text-xs text-muted-foreground">{t("settings:voiceEnableDescription")}</p>
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
  const { t } = useTranslation();
  if (caps.webSpeech || caps.whisperWeb || caps.audioCapture) return null;
  // Secure-context requirement is the most common reason capability detection
  // returns all-false on mobile (when reaching the dev server over LAN HTTP).
  // Spell it out so the user doesn't have to guess.
  const insecure = typeof window !== "undefined" && !window.isSecureContext;
  return (
    <div className="flex items-start gap-3 rounded-md border border-orange-500/40 bg-orange-500/5 p-3">
      <IconAlertTriangle className="h-5 w-5 text-orange-500 shrink-0 mt-0.5" />
      <div className="space-y-1 text-sm">
        <p className="font-medium">{t("settings:voiceUnavailableTitle")}</p>
        <p className="text-xs text-muted-foreground">
          {insecure
            ? t("settings:voiceUnavailableInsecure")
            : t("settings:voiceUnavailableUnsupported")}
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
  const { t } = useTranslation();
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
          {/* The shortcut's own label comes from the shared keyboard registry,
              which is still English — see the guard comment for this route. */}
          {t("settings:voiceShortcutTitle", {
            shortcut: CONFIGURABLE_SHORTCUTS.VOICE_INPUT_TOGGLE.label,
          })}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <ShortcutRecorder
          shortcutId="VOICE_INPUT_TOGGLE"
          label={CONFIGURABLE_SHORTCUTS.VOICE_INPUT_TOGGLE.label}
          defaultShortcut={CONFIGURABLE_SHORTCUTS.VOICE_INPUT_TOGGLE.default}
          current={current}
          onChange={handleChange}
          onReset={handleReset}
          isDirty={isDirty}
        />
        <p className="text-xs text-muted-foreground mt-2">{t("settings:voiceShortcutHint")}</p>
      </CardContent>
    </SettingsCard>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────

function VoiceModeSettingsContent() {
  const { t } = useTranslation();
  const caps = useMemo(() => detectVoiceCapabilities(), []);
  const { voiceMode } = useVoiceDraft();
  const enabled = voiceMode.enabled;
  return (
    <SettingsSection
      icon={<IconMicrophone className="h-5 w-5" />}
      title={t("settings:voiceMode")}
      description={t("settings:voiceModePageDescription")}
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
