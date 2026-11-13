/** The compiler must reject the unbounded instantiation and accept the
 * bounded counterpart. Both the pinned reference and fast tsrs must agree. */
type Recurse<T> = T extends unknown ? Recurse<T> : never;
export type Excessive = Recurse<string>;

type Bounded<T, Depth extends number = 4> = Depth extends 0
  ? T
  : Bounded<{ value: T }, [-1, 0, 1, 2, 3][Depth] & number>;
export type Finite = Bounded<number>;
