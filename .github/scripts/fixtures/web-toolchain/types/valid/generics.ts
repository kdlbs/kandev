/** Generic, conditional, mapped, recursive and inference shapes that must agree
 * between the reference compiler and the candidate. */
export type Awaitable<T> = T | Promise<T>;

export type Unwrap<T> = T extends Promise<infer Value>
  ? Unwrap<Value>
  : T extends readonly (infer Item)[]
    ? Item
    : T;

export type DeepPartial<T> = {
  [Key in keyof T]?: T[Key] extends object ? DeepPartial<T[Key]> : T[Key];
};

export type Tree<T> = {
  value: T;
  children: readonly Tree<T>[];
};

export function flatten<T>(tree: Tree<T>): T[] {
  return [tree.value, ...tree.children.flatMap((child) => flatten(child))];
}

export function firstItem<T>(values: readonly T[]): T | undefined {
  return values[0];
}

export function isPromise<T>(value: Awaitable<T>): value is Promise<T> {
  return typeof (value as Promise<T>).then === "function";
}

export function groupBy<Item, Key extends PropertyKey>(
  items: readonly Item[],
  keyOf: (item: Item) => Key,
): Map<Key, Item[]> {
  const grouped = new Map<Key, Item[]>();
  for (const item of items) {
    const key = keyOf(item);
    const bucket = grouped.get(key) ?? [];
    bucket.push(item);
    grouped.set(key, bucket);
  }
  return grouped;
}

export const unwrapNumber: Unwrap<Promise<number>> = 1;
export const firstNumber: number | undefined = firstItem([1, 2, 3]);
export const partialTree: DeepPartial<Tree<number>> = { value: 1, children: [{ value: 2 }] };
