/**
 * Returns the first value in a CSS animation list without splitting commas
 * inside functional notation such as cubic-bezier() or steps().
 */
function firstAnimationListValue(value: string): string | null {
  let parentheses = 0;
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index];
    if (character === "(") {
      parentheses += 1;
    } else if (character === ")") {
      parentheses = Math.max(0, parentheses - 1);
    } else if (character === "," && parentheses === 0) {
      const firstValue = value.slice(0, index).trim();
      return firstValue || null;
    }
  }

  const firstValue = value.trim();
  return firstValue || null;
}

function parseCssTime(value: string): number | null {
  const firstValue = firstAnimationListValue(value);
  if (!firstValue) return null;

  const numericValue = Number.parseFloat(firstValue);
  if (!Number.isFinite(numericValue)) return null;
  return firstValue.endsWith("ms") ? numericValue : numericValue * 1_000;
}

export { firstAnimationListValue, parseCssTime };
