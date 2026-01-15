'use client';

export function TypingIndicator() {
  return (
    <div className="flex items-center gap-2 px-4 py-3 max-w-[85%] rounded-lg bg-muted text-muted-foreground">
      <div className="flex items-center gap-1" role="status" aria-label="Agent is typing">
        <span className="text-[11px] uppercase tracking-wide opacity-70 mr-2">Agent</span>
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '0ms', animationDuration: '1s' }}
        />
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '150ms', animationDuration: '1s' }}
        />
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '300ms', animationDuration: '1s' }}
        />
      </div>
    </div>
  );
}
