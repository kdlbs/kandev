import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CommandItem } from "@/lib/commands/types";
import { activateLocale, DEFAULT_LOCALE } from "@/lib/i18n";
import { GlobalCommands } from "./global-commands";

const mocks = vi.hoisted(() => {
  return {
    commands: [] as CommandItem[],
    destinations: [
      {
        id: "stats",
        label: "Stats",
        icon: () => null,
        section: "insights" as const,
        href: "/stats",
        palette: {
          id: "nav-stats",
          labelKey: "common:commandGoToStats",
          keywordsKey: "common:commandGoToStatsKeywords",
        },
      },
    ],
    push: vi.fn(),
    quickChat: vi.fn(),
    quickChatLauncherCalls: [] as unknown[][],
    keyboardShortcutCalls: [] as unknown[][],
    setTheme: vi.fn(),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      userSettings: { keyboardShortcuts: {} },
      workspaces: { activeId: "workspace-1" },
    }),
}));
vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme: "dark", setTheme: mocks.setTheme }),
}));
vi.mock("@/hooks/use-app-destinations", () => ({
  useStaticDestinations: () => mocks.destinations.map((destination) => ({ ...destination })),
}));
vi.mock("@/hooks/use-app-shortcuts", () => ({ useAppShortcuts: vi.fn() }));
vi.mock("@/hooks/use-keyboard-shortcut", () => ({
  useKeyboardShortcut: (...args: unknown[]) => mocks.keyboardShortcutCalls.push(args),
}));
vi.mock("@/hooks/use-plugin-shortcuts", () => ({ usePluginShortcuts: vi.fn() }));
vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: (...args: unknown[]) => {
    mocks.quickChatLauncherCalls.push(args);
    return mocks.quickChat;
  },
}));
vi.mock("@/hooks/use-register-commands", () => ({
  useRegisterCommands: (commands: CommandItem[]) => {
    mocks.commands = commands;
  },
}));
vi.mock("@/lib/keyboard/shortcut-overrides", () => ({ getShortcut: () => undefined }));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
}));
vi.mock("@/components/settings-discovery-commands", () => ({
  SettingsDiscoveryCommands: () => null,
}));

function navigationCommand(): CommandItem {
  const command = mocks.commands.find((item) => item.id === "nav-stats");
  if (!command) throw new Error("Missing Stats navigation command");
  return command;
}

beforeEach(async () => {
  mocks.commands = [];
  mocks.quickChatLauncherCalls = [];
  mocks.keyboardShortcutCalls = [];
  mocks.push.mockReset();
  await activateLocale(DEFAULT_LOCALE);
});

afterEach(async () => {
  cleanup();
  await activateLocale(DEFAULT_LOCALE);
});

describe("GlobalCommands navigation commands", () => {
  it("maps destinations to commands and navigation actions", () => {
    render(<GlobalCommands />);

    const command = navigationCommand();
    expect(command).toMatchObject({
      id: "nav-stats",
      label: "Stats",
      group: "Navigation",
      keywords: ["stats", "statistics", "analytics", "metrics"],
    });

    command.action?.();
    expect(mocks.push).toHaveBeenCalledWith("/stats");
  });

  it("does not request silent focus for global quick chat commands", () => {
    render(<GlobalCommands />);

    expect(mocks.quickChatLauncherCalls).toEqual([
      ["workspace-1", "chat", { silentFocusReturn: false, toggleWhenOpen: true }],
      ["workspace-1", "config", { silentFocusReturn: false }],
    ]);
  });

  it("@covers AC-UI-QUICK-TERMINAL-001.10 captures the toggle before xterm can consume it", () => {
    render(<GlobalCommands />);

    expect(mocks.keyboardShortcutCalls).toHaveLength(1);
    expect(mocks.keyboardShortcutCalls[0]?.[2]).toEqual({
      capture: true,
      stopPropagation: true,
    });
  });

  // Drives a real locale switch rather than a stubbed `t`. The point of the
  // test is that the memo key tracks translated values, and only the real
  // catalog can show that end to end — a hand-written translation table proves
  // the memo reacts to the table, not to a locale change.
  it("keeps command identity until a translated command value changes", async () => {
    const { rerender } = render(<GlobalCommands />);
    const first = navigationCommand();

    rerender(<GlobalCommands />);
    expect(navigationCommand()).toBe(first);

    await act(async () => {
      await activateLocale("pt-pt");
    });
    rerender(<GlobalCommands />);

    const translated = navigationCommand();
    expect(translated).not.toBe(first);
    expect(translated.group).toBe("Navegação");
    expect(translated.keywords).toEqual(["estatísticas", "análises", "métricas"]);
  });
});
