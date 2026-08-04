import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CommandItem } from "@/lib/commands/types";
import { GlobalCommands } from "./global-commands";

const mocks = vi.hoisted(() => {
  const translations: Record<string, string> = {};
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
    setTheme: vi.fn(),
    translations,
    translate: vi.fn((key: string) => translations[key] ?? key),
  };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: mocks.translate }),
}));
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
vi.mock("@/hooks/use-keyboard-shortcut", () => ({ useKeyboardShortcut: vi.fn() }));
vi.mock("@/hooks/use-plugin-shortcuts", () => ({ usePluginShortcuts: vi.fn() }));
vi.mock("@/hooks/use-quick-chat-launcher", () => ({
  useQuickChatLauncher: () => mocks.quickChat,
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

function navigationCommand(): CommandItem {
  const command = mocks.commands.find((item) => item.id === "nav-stats");
  if (!command) throw new Error("Missing Stats navigation command");
  return command;
}

beforeEach(() => {
  mocks.commands = [];
  mocks.push.mockReset();
  mocks.translations["common:commandGroupNavigation"] = "Navigation";
  mocks.translations["common:commandGoToStatsKeywords"] = "stats, metrics";
});

afterEach(cleanup);

describe("GlobalCommands navigation commands", () => {
  it("maps destinations to commands and navigation actions", () => {
    render(<GlobalCommands />);

    const command = navigationCommand();
    expect(command).toMatchObject({
      id: "nav-stats",
      label: "Stats",
      group: "Navigation",
      keywords: ["stats", "metrics"],
    });

    command.action?.();
    expect(mocks.push).toHaveBeenCalledWith("/stats");
  });

  it("keeps command identity until a translated command value changes", () => {
    const { rerender } = render(<GlobalCommands />);
    const first = navigationCommand();

    rerender(<GlobalCommands />);
    expect(navigationCommand()).toBe(first);

    mocks.translations["common:commandGroupNavigation"] = "Navigation pseudo";
    mocks.translations["common:commandGoToStatsKeywords"] = "statistiques, mesures";
    rerender(<GlobalCommands />);

    const translated = navigationCommand();
    expect(translated).not.toBe(first);
    expect(translated.group).toBe("Navigation pseudo");
    expect(translated.keywords).toEqual(["statistiques", "mesures"]);
  });
});
