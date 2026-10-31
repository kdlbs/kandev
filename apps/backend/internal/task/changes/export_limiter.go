package changes

import "context"

const turnChangeExportConcurrency = 2

// ExportLimiter bounds content preservation to two active attempts per
// installation. Every foreground and background entry point shares one limiter.
type ExportLimiter struct {
	sem chan struct{}
}

func NewExportLimiter() *ExportLimiter {
	return &ExportLimiter{sem: make(chan struct{}, turnChangeExportConcurrency)}
}

func (l *ExportLimiter) Acquire(ctx context.Context) bool {
	if l == nil {
		return true
	}
	select {
	case l.sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (l *ExportLimiter) Release() {
	if l == nil {
		return
	}
	<-l.sem
}

func (c *Coordinator) SetExportLimiter(limiter *ExportLimiter) {
	if c != nil {
		c.exportLimiter = limiter
	}
}
