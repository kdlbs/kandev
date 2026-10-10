package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type TurnChangeSetSummaryDTO struct {
	ID                      string                        `json:"id"`
	TaskID                  string                        `json:"task_id"`
	SessionID               string                        `json:"session_id"`
	TurnID                  string                        `json:"turn_id"`
	Revision                int64                         `json:"revision"`
	Availability            models.TurnChangeAvailability `json:"availability"`
	Reason                  models.TurnChangeReason       `json:"reason,omitempty"`
	Complete                bool                          `json:"complete"`
	SummaryComplete         bool                          `json:"summary_complete"`
	ContentComplete         bool                          `json:"content_complete"`
	TurnOrdinal             int64                         `json:"turn_ordinal"`
	TerminalAt              *time.Time                    `json:"terminal_at,omitempty"`
	TerminalOutcome         string                        `json:"terminal_outcome,omitempty"`
	FinalAssistantMessageID string                        `json:"final_assistant_message_id,omitempty"`
	FallbackAnchor          string                        `json:"fallback_anchor"`
	FileCount               int64                         `json:"file_count"`
	AddedLines              *int64                        `json:"added_lines,omitempty"`
	DeletedLines            *int64                        `json:"deleted_lines,omitempty"`
	BinaryFileCount         int64                         `json:"binary_file_count"`
	UnknownCountFileCount   int64                         `json:"unknown_count_file_count"`
	RepositoryCount         int64                         `json:"repository_count"`
	RetainUntil             *time.Time                    `json:"retain_until,omitempty"`
	ExpiryReason            string                        `json:"expiry_reason,omitempty"`
	OverlapIntervals        []models.TurnChangeOverlap    `json:"overlap_intervals,omitempty"`
	Repositories            []TurnRepositoryChangeDTO     `json:"repositories"`
}

type TurnRepositoryChangeDTO struct {
	ID                  string                        `json:"id"`
	CheckoutID          string                        `json:"checkout_id"`
	RepositoryID        string                        `json:"repository_id,omitempty"`
	WorktreeID          string                        `json:"worktree_id,omitempty"`
	DisplayName         string                        `json:"display_name,omitempty"`
	RepositorySubpath   string                        `json:"repository_subpath,omitempty"`
	Availability        models.TurnChangeAvailability `json:"availability"`
	Reason              models.TurnChangeReason       `json:"reason,omitempty"`
	EnumerationComplete bool                          `json:"enumeration_complete"`
	ComparisonComplete  bool                          `json:"comparison_complete"`
	ContentComplete     bool                          `json:"content_complete"`
	OverlapIntervals    []models.TurnChangeOverlap    `json:"overlap_intervals,omitempty"`
}

type TurnFileChangeDTO struct {
	ID                  string                        `json:"id"`
	RepositoryChangeID  string                        `json:"repository_change_id"`
	CheckoutID          string                        `json:"checkout_id"`
	Path                string                        `json:"path"`
	OldPath             *string                       `json:"old_path,omitempty"`
	Kind                string                        `json:"kind"`
	OldMode             string                        `json:"old_mode,omitempty"`
	NewMode             string                        `json:"new_mode,omitempty"`
	Submodule           bool                          `json:"submodule,omitempty"`
	Binary              bool                          `json:"binary,omitempty"`
	AddedLines          *int64                        `json:"added_lines,omitempty"`
	DeletedLines        *int64                        `json:"deleted_lines,omitempty"`
	ContentAvailability models.TurnChangeAvailability `json:"content_availability"`
	ContentReason       models.TurnChangeReason       `json:"content_reason,omitempty"`
	ContentTruncated    bool                          `json:"content_truncated,omitempty"`
}

type TurnChangeHistoryResponse struct {
	ChangeSets []TurnChangeSetSummaryDTO `json:"change_sets"`
	Total      int                       `json:"total"`
	Offset     int                       `json:"offset"`
	Limit      int                       `json:"limit"`
	NextOffset *int                      `json:"next_offset,omitempty"`
}

type TurnChangeFilesResponse struct {
	Files      []TurnFileChangeDTO `json:"files"`
	Total      int                 `json:"total"`
	Offset     int                 `json:"offset"`
	Limit      int                 `json:"limit"`
	NextOffset *int                `json:"next_offset,omitempty"`
}

type TurnChangeContentResponse struct {
	FileChangeID string                          `json:"file_change_id"`
	Variant      models.TurnChangeContentVariant `json:"variant"`
	Content      []byte                          `json:"content"`
	Digest       string                          `json:"digest"`
	Truncated    bool                            `json:"truncated,omitempty"`
}

func TurnChangeSetSummaryFromModel(changeSet *models.TurnChangeSet, repositories []*models.TurnRepositoryChangeSet) TurnChangeSetSummaryDTO {
	dto := TurnChangeSetSummaryDTO{
		Repositories: make([]TurnRepositoryChangeDTO, 0, len(repositories)),
	}
	if changeSet != nil {
		dto.ID, dto.TaskID, dto.SessionID, dto.TurnID = changeSet.ID, changeSet.TaskID, changeSet.TaskSessionID, changeSet.TurnID
		dto.Revision, dto.Availability, dto.Reason = changeSet.Revision, changeSet.Availability, changeSet.Reason
		dto.Complete, dto.SummaryComplete, dto.ContentComplete = changeSet.Complete, changeSet.SummaryComplete, changeSet.ContentComplete
		dto.TurnOrdinal, dto.TerminalAt, dto.TerminalOutcome = changeSet.TurnOrdinal, changeSet.TerminalAt, changeSet.TerminalOutcome
		dto.FinalAssistantMessageID, dto.FallbackAnchor = changeSet.FinalAssistantMessageID, changeSet.FallbackAnchor
		dto.FileCount, dto.AddedLines, dto.DeletedLines = changeSet.FileCount, changeSet.AddedLines, changeSet.DeletedLines
		dto.BinaryFileCount, dto.UnknownCountFileCount, dto.RepositoryCount = changeSet.BinaryFileCount, changeSet.UnknownCountFileCount, changeSet.RepositoryCount
		dto.RetainUntil, dto.ExpiryReason, dto.OverlapIntervals = changeSet.RetainUntil, changeSet.ExpiryReason, changeSet.OverlapIntervals
	}
	for _, repository := range repositories {
		if repository == nil {
			continue
		}
		dto.Repositories = append(dto.Repositories, TurnRepositoryChangeDTO{
			ID: repository.ID, CheckoutID: repository.CheckoutID, RepositoryID: repository.RepositoryID,
			WorktreeID: repository.WorktreeID, DisplayName: repository.DisplayName,
			RepositorySubpath: repository.RepositorySubpath, Availability: repository.Availability,
			Reason: repository.Reason, EnumerationComplete: repository.EnumerationComplete,
			ComparisonComplete: repository.ComparisonComplete, ContentComplete: repository.ContentComplete,
			OverlapIntervals: repository.OverlapIntervals,
		})
	}
	return dto
}

func TurnFileChangeFromModel(file *models.TurnFileChange) TurnFileChangeDTO {
	if file == nil {
		return TurnFileChangeDTO{}
	}
	return TurnFileChangeDTO{
		ID: file.ID, RepositoryChangeID: file.RepositoryChangeID, CheckoutID: file.CheckoutID,
		Path: file.Path, OldPath: file.OldPath, Kind: file.Kind, OldMode: file.OldMode, NewMode: file.NewMode,
		Submodule: file.Submodule, Binary: file.Binary, AddedLines: file.AddedLines, DeletedLines: file.DeletedLines,
		ContentAvailability: file.ContentAvailability, ContentReason: file.ContentReason, ContentTruncated: file.ContentTruncated,
	}
}
