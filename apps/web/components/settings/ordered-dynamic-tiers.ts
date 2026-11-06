import type { DynamicAgentCandidate, DynamicAgentSelection } from "@/lib/types/agent-profile";

type TierPolicy = NonNullable<DynamicAgentSelection["tier"]>;
type Block = { head: string; policy: TierPolicy; index: number };
const defaultTier = (): TierPolicy => ({ mode: "order", on_failure: "same_tier_next" });

export function joinsPrevious(row: DynamicAgentCandidate): boolean {
  return row.policies.selection?.join_previous === true;
}

function blocks(rows: DynamicAgentCandidate[]): Map<string, Block> {
  const result = new Map<string, Block>();
  let current: Block = { head: "", policy: defaultTier(), index: 0 };
  rows.forEach((row, index) => {
    if (index === 0 || !joinsPrevious(row)) {
      current = {
        head: row.executionProfileId,
        policy: row.policies.selection?.tier ?? defaultTier(),
        index: current.index + 1,
      };
    }
    result.set(row.executionProfileId, current);
  });
  return result;
}

export function tierNumbers(rows: DynamicAgentCandidate[]): number[] {
  const grouped = blocks(rows);
  return rows.map((row) => grouped.get(row.executionProfileId)!.index);
}

function withOwnership(
  row: DynamicAgentCandidate,
  position: number,
  joined: boolean,
  policy: TierPolicy,
): DynamicAgentCandidate {
  const selection: DynamicAgentSelection = { ...row.policies.selection, join_previous: joined };
  if (joined) delete selection.tier;
  else selection.tier = { ...policy };
  return { ...row, position, policies: { ...row.policies, selection } };
}

function rebuild(
  rows: DynamicAgentCandidate[],
  previous: Map<string, Block>,
): DynamicAgentCandidate[] {
  return rows.map((row, index) => {
    const block = previous.get(row.executionProfileId)!;
    const above = index > 0 ? previous.get(rows[index - 1].executionProfileId) : undefined;
    return withOwnership(row, index, above?.head === block.head, block.policy);
  });
}

export function moveTierCandidate(
  rows: DynamicAgentCandidate[],
  index: number,
  direction: -1 | 1,
): DynamicAgentCandidate[] {
  const target = index + direction;
  if (index < 0 || target < 0 || target >= rows.length) return rows;
  const moved = [...rows];
  [moved[index], moved[target]] = [moved[target], moved[index]];
  return rebuild(moved, blocks(rows));
}

export function removeTierCandidate(
  rows: DynamicAgentCandidate[],
  index: number,
): DynamicAgentCandidate[] {
  return rebuild(
    rows.filter((_, position) => position !== index),
    blocks(rows),
  );
}

export function toggleTierJoin(
  rows: DynamicAgentCandidate[],
  position: number,
): DynamicAgentCandidate[] {
  if (position <= 0 || position >= rows.length) return rows;
  const original = blocks(rows);
  const joining = !joinsPrevious(rows[position]);
  const upper = original.get(rows[position - 1].executionProfileId)!;
  const block = original.get(rows[position].executionProfileId)!;
  return rows.map((row, index) => {
    const own = original.get(row.executionProfileId)!;
    const policy = joining && own.head === block.head ? upper.policy : own.policy;
    return withOwnership(
      row,
      index,
      index === position ? joining : index > 0 && joinsPrevious(row),
      policy,
    );
  });
}
