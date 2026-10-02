package coordinator

import (
	"context"
	"time"
)

// launchInfo says whether the conversation session has launched and, if so,
// when. Sources edited after that moment no longer match what the session
// runs.
type launchInfo struct {
	applies    bool
	unreadable bool
	startedAt  time.Time
}

func (c *ContainmentChecker) launchOf(ctx context.Context, co *Coordinator) launchInfo {
	if co == nil || co.ConversationTaskID == nil || *co.ConversationTaskID == "" {
		return launchInfo{}
	}
	if c.readers.Sessions == nil {
		return launchInfo{unreadable: true}
	}
	session, err := c.readers.Sessions.ConversationSession(ctx, *co.ConversationTaskID)
	if err != nil {
		return launchInfo{unreadable: true}
	}
	if session == nil || session.State == launchStateCreated {
		return launchInfo{}
	}
	return launchInfo{applies: true, startedAt: session.StartedAt}
}

// currency returns "" when the source is current, DetailChangedSinceLaunch when
// it changed after launch, and DetailUnreadable when the launch or the source
// timestamp cannot be read.
func (l launchInfo) currency(updatedAt time.Time) string {
	switch {
	case l.unreadable:
		return DetailUnreadable
	case !l.applies:
		return ""
	case updatedAt.IsZero():
		return DetailUnreadable
	case updatedAt.After(l.startedAt):
		return DetailChangedSinceLaunch
	}
	return ""
}
