# Events and recovery

Subscribe to the relative event path supplied by the canvas contract when a
view needs live updates. Treat events as hints that invalidate a read; the
next read remains the source of truth.

Reconnect with bounded backoff after a dropped connection. Avoid duplicate
subscriptions and stop timers when the page is hidden or disposed. Show a
stale-data or reconnecting indicator when the view cannot refresh.

A failed publish does not activate a partial release. Keep the current draft
available, show safe validation diagnostics, and retry only after the source or
manifest is corrected.
