/** Compare two string collections as sets, ignoring order and duplicates in order. */
export function areStringSetsEqual(left: string[] = [], right: string[] = []): boolean {
  if (left.length !== right.length) return false;
  const sortedLeft = [...left].sort();
  const sortedRight = [...right].sort();
  return sortedLeft.every((value, index) => value === sortedRight[index]);
}
