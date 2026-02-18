import { useCallback, useMemo, type ReactNode } from 'react';
import { useTheme } from 'next-themes';
import type { FileDiffOptions, SelectedLineRange, RenderHeaderMetadataProps } from '@pierre/diffs';
import { IconPlus } from '@tabler/icons-react';
import { FONT } from '@/lib/theme/colors';
import { useGlobalViewMode } from '@/hooks/use-global-view-mode';
import { useDiffHeaderToolbar } from './diff-header-toolbar';
import type { AnnotationMetadata } from './use-diff-annotation-renderer';

/** CSS overrides for the Pierre diff viewer, injected via unsafeCSS. */
const DIFF_UNSAFE_CSS = `
  pre[data-diffs] {
    background-color: var(--background) !important;
    --diffs-bg: var(--background) !important;
    --diffs-bg-context: var(--background) !important;
    --diffs-bg-buffer: var(--background) !important;
    --diffs-bg-separator: var(--card) !important;
    --diffs-bg-hover: var(--muted) !important;
    --diffs-fg: var(--foreground) !important;
    --diffs-fg-number: var(--muted-foreground) !important;
    --diffs-addition-color-override: rgb(var(--git-addition)) !important;
    --diffs-deletion-color-override: rgb(var(--git-deletion)) !important;
    --diffs-font-size: ${FONT.size}px !important;
    --diffs-font-family: ${FONT.mono} !important;
  }
  [data-change-icon] {
    width: 12px !important;
    height: 12px !important;
  }
  [data-diffs-header] {
    padding-inline: 12px !important;
    background: var(--card) !important;
  }
`;

type UseDiffOptionsArgs = {
  filePath: string;
  diff?: string;
  enableComments: boolean;
  showHeader: boolean;
  wordWrap: boolean;
  setWordWrap: (fn: (v: boolean) => boolean) => void;
  handleLineSelectionEnd: (range: SelectedLineRange | null) => void;
  onLineEnter: (props: { lineType?: string; lineNumber?: number; annotationSide?: string }) => void;
  onLineLeave: () => void;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
};

type UseDiffOptionsResult = {
  globalViewMode: string;
  options: FileDiffOptions<AnnotationMetadata>;
  renderHeaderMetadata: ((props: RenderHeaderMetadataProps) => ReactNode) | undefined;
  renderHoverUtility: () => ReactNode;
};

export function useDiffOptions(args: UseDiffOptionsArgs): UseDiffOptionsResult {
  const {
    filePath, diff, enableComments, showHeader, wordWrap,
    setWordWrap, handleLineSelectionEnd, onLineEnter, onLineLeave,
    onOpenFile, onRevert,
  } = args;

  const { resolvedTheme } = useTheme();
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();

  const toggleViewMode = useCallback(
    () => setGlobalViewMode(globalViewMode === 'split' ? 'unified' : 'split'),
    [globalViewMode, setGlobalViewMode]
  );

  const toggleWordWrap = useCallback(
    () => setWordWrap((v: boolean) => !v),
    [setWordWrap]
  );

  const renderHeaderMetadata = useDiffHeaderToolbar({
    filePath, diff, wordWrap,
    onToggleWordWrap: toggleWordWrap,
    viewMode: globalViewMode,
    onToggleViewMode: toggleViewMode,
    onOpenFile, onRevert,
  });

  const renderHoverUtility = useCallback(
    (): ReactNode => {
      if (!enableComments) return null;
      return (
        <div
          className="flex h-5 w-5 cursor-pointer items-center justify-center rounded border border-border bg-background text-muted-foreground hover:bg-accent hover:text-foreground"
          title="Add comment"
        >
          <IconPlus className="h-3 w-3" />
        </div>
      );
    },
    [enableComments]
  );

  const options = useMemo<FileDiffOptions<AnnotationMetadata>>(
    () => ({
      diffStyle: globalViewMode,
      themeType: resolvedTheme === 'dark' ? 'dark' : 'light',
      enableLineSelection: enableComments,
      hunkSeparators: 'simple',
      enableHoverUtility: enableComments,
      diffIndicators: 'none',
      onLineSelectionEnd: handleLineSelectionEnd,
      onLineEnter,
      onLineLeave,
      disableFileHeader: !showHeader,
      overflow: wordWrap ? 'wrap' : 'scroll',
      unsafeCSS: DIFF_UNSAFE_CSS,
    }),
    [globalViewMode, resolvedTheme, enableComments, showHeader, handleLineSelectionEnd, wordWrap, onLineEnter, onLineLeave]
  );

  return { globalViewMode, options, renderHeaderMetadata: showHeader ? renderHeaderMetadata : undefined, renderHoverUtility };
}
