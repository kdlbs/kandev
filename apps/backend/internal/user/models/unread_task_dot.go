package models

const (
	// UnreadTaskDotHideDelayDefaultSeconds is how long an opened task keeps its
	// unread dot when the user has not chosen a delay.
	UnreadTaskDotHideDelayDefaultSeconds = 2
	// UnreadTaskDotHideDelayMaxSeconds bounds the configurable delay.
	UnreadTaskDotHideDelayMaxSeconds = 60
)

// ValidUnreadTaskDotHideDelaySeconds reports whether seconds is an accepted
// unread task dot hide delay; zero hides the dot as soon as the task opens.
func ValidUnreadTaskDotHideDelaySeconds(seconds int) bool {
	return seconds >= 0 && seconds <= UnreadTaskDotHideDelayMaxSeconds
}
