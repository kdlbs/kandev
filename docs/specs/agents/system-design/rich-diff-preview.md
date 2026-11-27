---
status: current
system: agents
requirements:
  - REQ-AGENTS-RICH-DIFF-PREVIEW-001
---

# Rich diff preview design

## Purpose and boundaries

Extend the native text branch of FilePreview in the shared rich-output renderer. The persisted spec continues to carry the workspace path and optional repo, title and caption. Agents use the existing version 2 catalog. Document the supported extensions in catalog guidance and the public authoring example.

## Rendering and access

After the existing lazy authorized read succeeds, select the diff view for text with a case-insensitive .diff or .patch suffix and a recognizable unified hunk. Preserve the original lines, including git metadata and the missing-final-newline marker. Identify file headers outside hunk bodies. Inside a hunk use the old and new line counts to distinguish content beginning with repeated signs from the next file header. Render escaped text nodes and keep paths inside the patch inert.

Place a localized two-option layout control above the patch. Expand each new native diff card automatically and default to Side by side on all screen sizes. Expose the selection to assistive technology and support keyboard and touch input. Store the selection locally in the card. Switching reuses the loaded patch.

In Side by side, label the columns Before and After. Mirror context on both sides. Within each contiguous change block, pair removed and added lines by ordinal position and leave unmatched cells empty. File and hunk headers span the columns. In Unified, show the original patch sequence. Both layouts use existing diff colors, literal signs and monospace alignment.

Build both layouts from the same source lines. Page in groups of 100 display rows for the active layout. Map the first visible change by source line to its row in the target layout so switching retains the reader's place, including across page boundaries. Mount only the active page. Reset to the first page on a fresh read. Keep horizontal overflow inside the patch and keep layout and paging controls in reach outside that region. Use touch targets of at least 44 px on phones and coarse pointers.

## Failure and lifecycle

Use the ordinary text branch for unsupported formats. Preserve the existing loading, retry, binary and plugin paths. The preview can only page through content returned by the existing reader. Keep the full-file action accessible. Add no file reads based on patch headers, storage, or content logging.

## Requirement mapping

REQ-AGENTS-RICH-DIFF-PREVIEW-001 is implemented at the existing file preview boundary and verified through the [delivery plan](../../../plans/rich-diff-preview/plan.md).
