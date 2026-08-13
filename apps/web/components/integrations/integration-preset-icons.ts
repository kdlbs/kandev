import {
  IconBug,
  IconChecks,
  IconCode,
  IconEye,
  IconMessageDots,
  IconSearch,
  IconSparkles,
  IconTool,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";

export type IntegrationPresetIconName =
  | "eye"
  | "message"
  | "tool"
  | "code"
  | "search"
  | "bug"
  | "sparkle"
  | "check";

const ICON_BY_NAME: Record<IntegrationPresetIconName, Icon> = {
  eye: IconEye,
  message: IconMessageDots,
  tool: IconTool,
  code: IconCode,
  search: IconSearch,
  bug: IconBug,
  sparkle: IconSparkles,
  check: IconChecks,
};

export function iconForIntegrationPreset(name: string | undefined): Icon {
  if (!name) return IconSparkles;
  return ICON_BY_NAME[name as IntegrationPresetIconName] ?? IconSparkles;
}
