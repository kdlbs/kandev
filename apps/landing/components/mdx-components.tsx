import type { ComponentProps, ReactElement } from 'react';
import { ChangelogVideo } from '@/components/changelog-video';

function MdxHeading({ className, ...props }: ComponentProps<'h2'>) {
  return <h2 className={`mt-10 text-2xl font-semibold text-foreground ${className ?? ''}`} {...props} />;
}

function MdxSubheading({ className, ...props }: ComponentProps<'h3'>) {
  return <h3 className={`mt-8 text-xl font-semibold text-foreground ${className ?? ''}`} {...props} />;
}

function MdxParagraph({ className, ...props }: ComponentProps<'p'>) {
  return <p className={`mt-4 text-base leading-relaxed text-muted-foreground ${className ?? ''}`} {...props} />;
}

function MdxList({ className, ...props }: ComponentProps<'ul'>) {
  return <ul className={`mt-4 space-y-2 pl-6 text-muted-foreground ${className ?? ''}`} {...props} />;
}

function MdxListItem({ className, ...props }: ComponentProps<'li'>) {
  return <li className={`list-disc text-base ${className ?? ''}`} {...props} />;
}

function MdxImage({ className, alt, ...props }: ComponentProps<'img'>): ReactElement {
  return (
    <img
      className={`mt-6 w-full rounded-2xl border border-border bg-muted/40 ${className ?? ''}`}
      alt={alt ?? ''}
      loading="lazy"
      decoding="async"
      {...props}
    />
  );
}

export const mdxComponents = {
  h2: MdxHeading,
  h3: MdxSubheading,
  p: MdxParagraph,
  ul: MdxList,
  li: MdxListItem,
  img: MdxImage,
  ChangelogVideo,
};
