import type { Draft } from "immer";

/** Slice setters can update their fields but cannot replace the entire app store. */
export type SliceSet<S> = (
  update: S | Partial<S> | ((state: Draft<S>) => void),
  replace?: false,
) => void;

export type SliceCreator<S> = (set: SliceSet<S>, get: () => S) => S;
