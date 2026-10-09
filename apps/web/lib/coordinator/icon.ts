import { IconUserCog } from "@tabler/icons-react";

/**
 * The icon that stands for a coordinator, everywhere one is listed: the
 * sidebar rows, the phone nav, and the workspace settings tab.
 *
 * Deliberately not `IconRobot`, which settings already uses for agent
 * profiles (`agent-card.tsx`, the workflow step selectors). A coordinator
 * drawn as a robot reads as one more agent profile in a list that contains
 * both.
 */
export const CoordinatorIcon = IconUserCog;
