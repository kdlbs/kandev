import { describe, it, expect } from "vitest";
import {
  buildCoreFields,
  mapUserSettingsResponse,
  parseChangesPanelLayout,
  parseVoiceMode,
} from "./user-settings";

describe("buildCoreFields", () => {
  it("maps terminal_font_family to terminalFontFamily", () => {
    const settings = {
      workspace_id: "",
      workflow_filter_id: "",
      kanban_view_mode: "",
      repository_ids: [],
      preferred_shell: "",
      default_editor_id: "",
      enable_preview_on_click: false,
      chat_submit_key: "cmd_enter",
      review_auto_mark_on_scroll: true,
      show_release_notification: true,
      release_notes_last_seen_version: "",
      saved_layouts: [],
      default_utility_agent_id: "",
      default_utility_model: "",
      keyboard_shortcuts: {},
      terminal_link_behavior: "new_tab",
      terminal_font_family: "JetBrains Mono",
      updated_at: "2026-01-01T00:00:00Z",
    } as unknown as Parameters<typeof buildCoreFields>[0];

    const result = buildCoreFields(settings);
    expect(result.terminalFontFamily).toBe("JetBrains Mono");
  });

  it("returns null when terminal_font_family is empty", () => {
    const settings = {
      workspace_id: "",
      workflow_filter_id: "",
      kanban_view_mode: "",
      repository_ids: [],
      preferred_shell: "",
      default_editor_id: "",
      enable_preview_on_click: false,
      chat_submit_key: "cmd_enter",
      review_auto_mark_on_scroll: true,
      show_release_notification: true,
      release_notes_last_seen_version: "",
      saved_layouts: [],
      default_utility_agent_id: "",
      default_utility_model: "",
      keyboard_shortcuts: {},
      terminal_link_behavior: "new_tab",
      terminal_font_family: "",
      updated_at: "2026-01-01T00:00:00Z",
    } as unknown as Parameters<typeof buildCoreFields>[0];

    const result = buildCoreFields(settings);
    expect(result.terminalFontFamily).toBeNull();
  });
});

describe("buildTerminalFields via buildCoreFields", () => {
  it("maps terminal_font_size to terminalFontSize", () => {
    const settings = {
      terminal_font_size: 16,
      terminal_font_family: "",
      terminal_link_behavior: "new_tab",
    } as unknown as Parameters<typeof buildCoreFields>[0];

    const result = buildCoreFields(settings);
    expect(result.terminalFontSize).toBe(16);
  });

  it("returns null when terminal_font_size is 0", () => {
    const settings = {
      terminal_font_size: 0,
      terminal_font_family: "",
      terminal_link_behavior: "new_tab",
    } as unknown as Parameters<typeof buildCoreFields>[0];

    const result = buildCoreFields(settings);
    expect(result.terminalFontSize).toBeNull();
  });
});

describe("mapUserSettingsResponse", () => {
  it("returns null terminalFontFamily when response is null", () => {
    const result = mapUserSettingsResponse(null);
    expect(result.terminalFontFamily).toBeNull();
  });

  it("defaults changesPanelLayout to flat when response is null", () => {
    const result = mapUserSettingsResponse(null);
    expect(result.changesPanelLayout).toBe("flat");
  });
});

describe("parseChangesPanelLayout", () => {
  it('returns "tree" for "tree"', () => {
    expect(parseChangesPanelLayout("tree")).toBe("tree");
  });

  it('returns "flat" for "flat", undefined, or unknown values', () => {
    expect(parseChangesPanelLayout("flat")).toBe("flat");
    expect(parseChangesPanelLayout(undefined)).toBe("flat");
    expect(parseChangesPanelLayout("grid")).toBe("flat");
    expect(parseChangesPanelLayout("")).toBe("flat");
  });
});

describe("parseVoiceMode", () => {
  it("maps every field from the snake_case wire payload", () => {
    expect(
      parseVoiceMode({
        enabled: false,
        engine: "whisperWeb",
        language: "pt-PT",
        mode: "hold",
        auto_send: true,
        whisper_web_model: "small",
      }),
    ).toEqual({
      enabled: false,
      engine: "whisperWeb",
      language: "pt-PT",
      mode: "hold",
      autoSend: true,
      whisperWebModel: "small",
    });
  });

  it("returns the defaults when the payload is undefined", () => {
    expect(parseVoiceMode(undefined)).toEqual({
      enabled: true,
      engine: "auto",
      language: "auto",
      mode: "toggle",
      autoSend: false,
      whisperWebModel: "base",
    });
  });

  it("defaults enabled to true when the wire payload omits the field (old rows)", () => {
    const result = parseVoiceMode({
      engine: "auto",
      language: "auto",
      mode: "toggle",
      auto_send: false,
      whisper_web_model: "base",
    } as unknown as Parameters<typeof parseVoiceMode>[0]);
    expect(result.enabled).toBe(true);
  });

  it("fills in defaults for missing string fields and coerces auto_send to false", () => {
    const result = parseVoiceMode({
      engine: "" as unknown as "auto",
      language: "",
      mode: "" as unknown as "toggle",
      whisper_web_model: "" as unknown as "base",
    } as unknown as Parameters<typeof parseVoiceMode>[0]);
    expect(result).toEqual({
      enabled: true,
      engine: "auto",
      language: "auto",
      mode: "toggle",
      autoSend: false,
      whisperWebModel: "base",
    });
  });
});

describe("mapUserSettingsResponse voice mode", () => {
  it("defaults the whole voiceMode object when response is null", () => {
    const result = mapUserSettingsResponse(null);
    expect(result.voiceMode).toEqual({
      enabled: true,
      engine: "auto",
      language: "auto",
      mode: "toggle",
      autoSend: false,
      whisperWebModel: "base",
    });
  });
});
