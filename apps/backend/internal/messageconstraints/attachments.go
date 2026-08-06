// Package messageconstraints contains limits shared by message admission
// paths without making either the task model or queue implementation depend
// on the other.
package messageconstraints

const (
	// MaxAttachmentBytes is the maximum total payload size accepted for one
	// message.
	MaxAttachmentBytes int64 = 100 * 1024 * 1024
	// MaxAttachmentCount is the maximum number of attachments accepted for one
	// message.
	MaxAttachmentCount = 10
)
