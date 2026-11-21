// Mirrors internal/coordinator/standing_orders.go's Order wire shape. number
// is the 1-based active position and null for a retired order; created_by is
// the creator's user id (empty with auth off).
export type StandingOrder = {
  id: string;
  number: number | null;
  text: string;
  created_at: string;
  created_by: string;
  retired_at: string | null;
  last_applied_at: string | null;
};

export type StandingOrderListResponse = {
  orders: StandingOrder[];
};

export type AddStandingOrderRequest = {
  text: string;
  source_proposal_id?: string;
};

export type GoalCriterion = {
  id: string;
  text: string;
  done: boolean;
};

// Mirrors internal/coordinator/reads_phase2.go's Goal. due_on is a calendar
// date (YYYY-MM-DD) as stored; set_at and met_at are timestamps.
export type Goal = {
  id: string;
  coordinator_id: string;
  name: string;
  due_on: string | null;
  status: "active" | "met" | (string & {});
  criteria: GoalCriterion[];
  baseline: unknown;
  set_at: string;
  met_at: string | null;
  met_by: string | null;
  created_at: string;
  updated_at: string;
};

export type MeasureDirection = "up" | "down" | "none_small" | "none_no_baseline";

export type GoalMeasure = {
  current: number;
  baseline: number | null;
  direction: MeasureDirection;
};

export type GoalMeasures = {
  open_tasks: GoalMeasure;
  approved_7d: GoalMeasure;
  rejected_7d: GoalMeasure;
};

export type GoalResponse = {
  active: Goal | null;
  last_met: Goal | null;
  measures: GoalMeasures | null;
};

export type PutGoalRequest = {
  goal_id?: string;
  name: string;
  due_on?: string | null;
  criteria: { id?: string; text: string }[];
};
