export type MarkdownSourceReplacement = {
  start: number;
  endExclusive: number;
  newText: string;
};

/**
 * Applies editor replacements without normalizing the surrounding Markdown.
 * The upstream editor reports source offsets, so changing one block must not
 * rebuild or otherwise reinterpret the rest of the document.
 */
export function applyMarkdownSourceEdit(
  source: string,
  replacements: readonly MarkdownSourceReplacement[],
): string {
  validateReplacements(source, replacements);

  return replacements.reduceRight(
    (result, replacement) =>
      `${result.slice(0, replacement.start)}${replacement.newText}${result.slice(replacement.endExclusive)}`,
    source,
  );
}

function validateReplacements(
  source: string,
  replacements: readonly MarkdownSourceReplacement[],
): void {
  let previousEnd = 0;

  for (const replacement of replacements) {
    if (
      !Number.isInteger(replacement.start) ||
      !Number.isInteger(replacement.endExclusive) ||
      replacement.start < 0 ||
      replacement.endExclusive < replacement.start ||
      replacement.endExclusive > source.length
    ) {
      throw new RangeError("Markdown source edit is outside the source document");
    }

    if (replacement.start < previousEnd) {
      throw new RangeError("Markdown source edits must not overlap");
    }

    previousEnd = replacement.endExclusive;
  }
}
