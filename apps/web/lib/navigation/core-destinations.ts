/**
 * The app's first-party navigation catalog: one declaration per top-level
 * destination, in the order surfaces should offer them.
 *
 * Why this exists: each surface used to hardcode its own list. Adding a
 * destination meant editing three unrelated files, and forgetting one silently
 * dropped it from that surface — `/stats` was reachable only from the
 * desktop-only sidebar footer and a keyboard-only palette command, so it had no
 * phone entry point at all. `core-destinations.test.ts` now fails when a
 * first-class route has no manifest entry, or when a destination is missing from
 * the mobile menu without a recorded reason.
 *
 * What this manifest currently drives: the sidebar's integrations section and
 * footer insight buttons, the sidebar and mobile plugin nav groups, the mobile
 * menu's integrations and utility groups, and the command palette's Navigation
 * group.
 *
 * What it does not drive yet (deliberately, see the notes below): the sidebar's
 * primary nav and the mobile header/View-toggle affordances for Home and Tasks,
 * action-shaped controls (New task, Inbox, quick chat, the settings gear, the
 * Office↔Kanban switch), the settings tree, and Office's own navigation section.
 * Those stay bespoke until the shared-shell work lands.
 */
import {
  IconBrandGithub,
  IconBrandGitlab,
  IconChartBar,
  IconHexagon,
  IconHome,
  IconList,
  IconSettings,
  IconTicket,
} from "@tabler/icons-react";
import { AzureDevOpsIcon } from "@/components/icons/azure-devops-icon";
import { linkToTaskOverview, linkToTasks } from "@/lib/links";
import { EVERYWHERE, MENU_AND_PALETTE, PALETTE_ONLY, SIDEBAR_AND_MENU } from "./surface-policy";
import type { Destination } from "./types";

/**
 * Not listed here, on purpose:
 * - Home/Inbox/New task in the sidebar's primary nav and the Office↔Kanban
 *   toggle — those carry badge counts, quick-chat launches, or `localStorage`
 *   side effects. They are actions, not plain destinations; folding actions in
 *   is a follow-up that should reuse the command registry.
 * - `/settings/<section>` deep links (the palette's Settings group) — those are
 *   shortcuts into one destination, not destinations of their own.
 * - `/login`, `/setup`, `/invite` — pre-auth routes, intentionally unlisted.
 */
export const APP_DESTINATIONS: Destination[] = [
  {
    id: "home",
    labelKey: "sidebar:home",
    icon: IconHome,
    section: "primary",
    href: (ctx) =>
      ctx.inOffice ? "/office" : linkToTaskOverview({ workspaceId: ctx.workspaceId ?? undefined }),
    // Palette-only: the sidebar's primary nav and the mobile header's brand link
    // already own "go home", so a second entry would duplicate them.
    surfaces: PALETTE_ONLY,
    palette: {
      id: "nav-home",
      labelKey: "common:commandGoToHome",
      keywordsKey: "common:commandGoToHomeKeywords",
      // The palette has always sent users to the workspace-less overview; a
      // manifest refactor is the wrong place to change where a command lands.
      href: () => linkToTaskOverview(),
    },
  },
  {
    id: "tasks",
    labelKey: "sidebar:tasks",
    icon: IconList,
    section: "primary",
    href: (ctx) => linkToTasks(ctx.workspaceId ?? undefined),
    // Palette-only: the sidebar's Tasks section and the mobile menu's View
    // toggle are the established ways to reach the task list.
    surfaces: PALETTE_ONLY,
    palette: {
      id: "nav-tasks",
      labelKey: "common:commandGoToAllTasks",
      keywordsKey: "common:commandGoToAllTasksKeywords",
      href: "/tasks",
    },
  },
  {
    id: "stats",
    labelKey: "sidebar:stats",
    icon: IconChartBar,
    section: "insights",
    href: "/stats",
    surfaces: EVERYWHERE,
    palette: {
      id: "nav-stats",
      labelKey: "common:commandGoToStats",
      keywordsKey: "common:commandGoToStatsKeywords",
    },
  },
  {
    id: "settings",
    labelKey: "common:settings",
    icon: IconSettings,
    section: "utilities",
    href: "/settings",
    // The sidebar's gear also toggles the sidebar's settings takeover, so it
    // stays bespoke; this entry serves the mobile menu and the palette.
    surfaces: MENU_AND_PALETTE,
    palette: {
      id: "nav-settings",
      labelKey: "common:commandGoToSettings",
      keywordsKey: "common:commandGoToSettingsKeywords",
      href: "/settings/general",
    },
  },
  {
    id: "azure-devops",
    label: "Azure DevOps",
    icon: AzureDevOpsIcon,
    section: "integrations",
    href: "/azure-devops",
    // Integrations other than GitHub have no palette command yet: the palette
    // needs "Go to <product>" copy, and adding it means new catalog keys. Adding
    // `palette` here plus an override block is all it takes.
    surfaces: SIDEBAR_AND_MENU,
    requires: "azure-devops",
  },
  {
    id: "github",
    label: "GitHub",
    icon: IconBrandGithub,
    section: "integrations",
    href: "/github",
    surfaces: EVERYWHERE,
    requires: "github",
    palette: {
      id: "nav-github",
      labelKey: "common:commandGoToGitHubDashboard",
      keywordsKey: "common:commandGoToGitHubDashboardKeywords",
      // The GitHub command predates per-workspace availability checks and has
      // always been listed unconditionally; the dashboard itself explains how to
      // connect. Kept as-is so this refactor changes no behavior.
      ignoreRequires: true,
    },
  },
  {
    id: "gitlab",
    label: "GitLab",
    icon: IconBrandGitlab,
    section: "integrations",
    href: "/gitlab",
    surfaces: SIDEBAR_AND_MENU,
    requires: "gitlab",
  },
  {
    id: "jira",
    label: "Jira",
    icon: IconTicket,
    section: "integrations",
    href: "/jira",
    surfaces: SIDEBAR_AND_MENU,
    requires: "jira",
  },
  {
    id: "linear",
    label: "Linear",
    icon: IconHexagon,
    section: "integrations",
    href: "/linear",
    surfaces: SIDEBAR_AND_MENU,
    requires: "linear",
  },
];
