export const SETUP_WIZARD_STEPS = {
  WORKSPACE: 0,
  TIER_PROFILES: 1,
  AGENT: 2,
  TASK: 3,
  REVIEW: 4,
} as const;

export const SETUP_WIZARD_STEP_COUNT = Object.keys(SETUP_WIZARD_STEPS).length;
