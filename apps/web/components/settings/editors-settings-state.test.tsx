import { act, cleanup, render, renderHook, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { activateLocale, DEFAULT_LOCALE, t } from "@/lib/i18n";
import type { EditorOption } from "@/lib/types/http";
import { getCustomEditorSummary, getCustomKindLabel } from "./editor-form";
import {
  buildDefaultEditorOptions,
  useLspConfigActions,
  useLspLanguageToggles,
} from "./editors-settings-state";

afterEach(async () => {
  cleanup();
  // Leave the shared instance on `en` for the suites that assert English copy.
  await activateLocale(DEFAULT_LOCALE);
});

const translate = (key: string) => `t(${key})`;

function editor(overrides: Partial<EditorOption> = {}): EditorOption {
  return {
    id: "editor-1",
    name: "VS Code",
    kind: "custom_command",
    config: {},
    enabled: true,
    installed: true,
    ...overrides,
  } as EditorOption;
}

describe("getCustomKindLabel", () => {
  it("resolves each known kind through the catalog", () => {
    expect(getCustomKindLabel(translate, "custom_command")).toBe("t(settings:command)");
    expect(getCustomKindLabel(translate, "custom_remote_ssh")).toBe("t(settings:vsCodeRemoteSsh)");
    expect(getCustomKindLabel(translate, "custom_hosted_url")).toBe("t(settings:hostedUrl)");
  });

  it("falls back to the generic label for an unknown kind", () => {
    expect(getCustomKindLabel(translate, "built_in")).toBe("t(settings:customEditorType)");
  });
});

describe("getCustomEditorSummary", () => {
  it("shows the configured value verbatim, never translated", () => {
    expect(getCustomEditorSummary(translate, editor({ config: { command: "code {file}" } }))).toBe(
      "code {file}",
    );
    expect(
      getCustomEditorSummary(
        translate,
        editor({ kind: "custom_hosted_url", config: { url: "https://code.example.com" } }),
      ),
    ).toBe("https://code.example.com");
    expect(
      getCustomEditorSummary(
        translate,
        editor({ kind: "custom_remote_ssh", config: { host: "box.example.com", user: "ada" } }),
      ),
    ).toBe("ada@box.example.com");
  });

  it("falls back to the translated kind label when nothing is configured", () => {
    expect(getCustomEditorSummary(translate, editor({ config: {} }))).toBe("t(settings:command)");
  });

  // A user without a host is not addressable, so the summary shows the kind
  // rather than a bare username that looks like a target.
  it("falls back to the kind label when host is absent, even if user is set", () => {
    expect(
      getCustomEditorSummary(
        translate,
        editor({ kind: "custom_remote_ssh", config: { user: "ada" } }),
      ),
    ).toBe("t(settings:vsCodeRemoteSsh)");
  });
});

describe("buildDefaultEditorOptions", () => {
  it("keys options by editor id and translates only the badge", () => {
    const options = buildDefaultEditorOptions(
      [editor({ id: "vscode", kind: "built_in", name: "VS Code", installed: false })],
      "vscode",
      // The real `t`, so a missing catalog entry fails here rather than shipping.
      t,
    );

    // `value` is submitted and compared with `===` — it must stay the raw id.
    expect(options[0].value).toBe("vscode");
    expect(options[0].label).toBe("VS Code");
    render(<>{options[0].renderLabel?.()}</>);
    expect(screen.getByText("Not installed")).toBeTruthy();
  });

  it("keeps the option value stable when the locale changes", async () => {
    const editors = [editor({ id: "vscode", kind: "built_in" })];
    const english = buildDefaultEditorOptions(editors, "vscode", (key) => key);
    await act(async () => {
      await activateLocale("pseudo");
    });
    const pseudo = buildDefaultEditorOptions(editors, "vscode", (key) => key);

    expect(pseudo[0].value).toBe(english[0].value);
    expect(pseudo[0].value).toBe("vscode");
  });
});

describe("useLspLanguageToggles", () => {
  function trackedState() {
    const autoStart: Array<(prev: string[]) => string[]> = [];
    const autoInstall: Array<(prev: string[]) => string[]> = [];
    return {
      calls: { autoStart, autoInstall },
      state: {
        setLspAutoStartLanguages: (updater: (prev: string[]) => string[]) =>
          autoStart.push(updater),
        setLspAutoInstallLanguages: (updater: (prev: string[]) => string[]) =>
          autoInstall.push(updater),
      } as unknown as Parameters<typeof useLspLanguageToggles>[0],
    };
  }

  it("adds a language when checked and removes it when unchecked", () => {
    const { calls, state } = trackedState();
    const { result } = renderHook(() => useLspLanguageToggles(state));

    result.current.toggleAutoStart("go", true);
    expect(calls.autoStart[0](["rust"])).toEqual(["rust", "go"]);

    result.current.toggleAutoStart("go", false);
    expect(calls.autoStart[1](["rust", "go"])).toEqual(["rust"]);

    result.current.toggleAutoInstall("python", true);
    expect(calls.autoInstall[0]([])).toEqual(["python"]);
  });
});

describe("useLspConfigActions", () => {
  function collectErrors() {
    let errors: Record<string, string> = {};
    let configs: Record<string, string> = {};
    const hook = renderHook(() =>
      useLspConfigActions(
        (updater) => {
          configs = updater(configs);
        },
        (updater) => {
          errors = updater(errors);
        },
      ),
    );
    return { hook, read: () => errors };
  }

  it("reports invalid JSON through the catalog rather than a literal", () => {
    const { hook, read } = collectErrors();
    hook.result.current.updateLspConfigString("go", "{ not json");
    expect(read().go).toBe("Invalid JSON");
  });

  it("rejects a non-object JSON value", () => {
    const { hook, read } = collectErrors();
    hook.result.current.updateLspConfigString("go", "[1, 2]");
    expect(read().go).toBe("Must be a JSON object");
  });

  it("clears the error once the value parses", () => {
    const { hook, read } = collectErrors();
    hook.result.current.updateLspConfigString("go", "{ not json");
    hook.result.current.updateLspConfigString("go", '{ "gopls": {} }');
    expect(read().go).toBeUndefined();
  });
});
