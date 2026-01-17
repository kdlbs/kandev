import type { HTMLAttributes } from 'react';

type ChangelogVideoProps = {
  src: string;
  poster?: string;
  caption?: string;
} & HTMLAttributes<HTMLVideoElement>;

export function ChangelogVideo({ src, poster, caption, className, ...props }: ChangelogVideoProps) {
  return (
    <figure className="mt-6">
      <video
        className={`w-full rounded-2xl border border-border bg-muted/40 ${className ?? ''}`}
        src={src}
        poster={poster}
        autoPlay
        loop
        muted
        playsInline
        preload="metadata"
        {...props}
      />
      {caption ? (
        <figcaption className="mt-2 text-sm text-muted-foreground">{caption}</figcaption>
      ) : null}
    </figure>
  );
}
