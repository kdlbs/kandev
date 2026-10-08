package models

import "github.com/google/uuid"

// ExecutorFailureMessageID binds one immutable history fact to one affected execution.
func ExecutorFailureMessageID(episodeID, sessionID, executionID string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("executor-failure:"+episodeID+":"+sessionID+":"+executionID)).String()
}
