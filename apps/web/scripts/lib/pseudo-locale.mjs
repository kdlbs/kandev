const MAP = {
  a: "à",
  b: "ƀ",
  c: "ć",
  d: "ď",
  e: "ē",
  f: "ƒ",
  g: "ĝ",
  h: "ĥ",
  i: "ĩ",
  j: "ĵ",
  k: "ķ",
  l: "ĺ",
  m: "ḿ",
  n: "ń",
  o: "ō",
  p: "ƥ",
  q: "q",
  r: "ŕ",
  s: "ś",
  t: "ţ",
  u: "ũ",
  v: "v",
  w: "ŵ",
  x: "x",
  y: "ŷ",
  z: "ź",
  A: "À",
  B: "Ɓ",
  C: "Ć",
  D: "Ď",
  E: "Ē",
  F: "Ƒ",
  G: "Ĝ",
  H: "Ĥ",
  I: "Ĩ",
  J: "Ĵ",
  K: "Ķ",
  L: "Ĺ",
  M: "Ḿ",
  N: "Ń",
  O: "Ō",
  P: "Ƥ",
  Q: "Q",
  R: "Ŕ",
  S: "Ś",
  T: "Ţ",
  U: "Ũ",
  V: "V",
  W: "Ŵ",
  X: "X",
  Y: "Ŷ",
  Z: "Ź",
};

/** Accent letters outside placeholders, tags, and Markdown code spans. */
function pseudolocalize(text) {
  // Split on placeholders/tags/code spans so their contents survive untouched.
  const parts = text.split(/(`[^`]*`|\{\{[^}]*\}\}|<\/?\d+>)/g);
  return parts
    .map((part, i) => (i % 2 === 1 ? part : part.replace(/[A-Za-z]/g, (c) => MAP[c] ?? c)))
    .join("");
}

export function transform(value) {
  if (typeof value === "string") return pseudolocalize(value);
  if (Array.isArray(value)) return value.map(transform);
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, transform(v)]));
  }
  return value;
}
