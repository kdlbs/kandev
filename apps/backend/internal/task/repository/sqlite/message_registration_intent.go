package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/task/models"
	userstore "github.com/kandev/kandev/internal/user/store"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	messageRegistrationProviderGitHub = "github"
	messageRegistrationProviderGitLab = "gitlab"
)

func (r *Repository) createMessageRegistrationIntentsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	messageType, metadataJSON string,
) error {
	if message.AuthorType != models.MessageAuthorUser || messageType != string(models.MessageTypeMessage) || metadataJSON == "" {
		return nil
	}
	var metadata struct {
		EntityReferences      []apiv1.EntityReference `json:"entity_references"`
		RegistrationOriginIDs []string                `json:"registration_origin_ids"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return fmt.Errorf("decode message references for registration: %w", err)
	}
	transferredSelections, err := r.transferQueueRegistrationOriginsTx(
		ctx, tx, message, metadata.RegistrationOriginIDs,
	)
	if err != nil {
		return err
	}
	_, err = r.projectTransferredQueueReferencesTx(ctx, tx, message, metadata.EntityReferences, transferredSelections)
	if err != nil {
		return err
	}
	if len(metadata.EntityReferences) == 0 {
		return nil
	}
	references, err := entityrefs.NormalizeForSubmission(metadata.EntityReferences)
	if err != nil {
		return fmt.Errorf("validate accepted message references: %w", err)
	}
	for _, reference := range references {
		if reference.AssociationMode != apiv1.EntityReferenceModeLinkOnSend {
			continue
		}
		if _, transferred := transferredSelections[reference.SelectionID]; transferred {
			continue
		}
		if err := r.createMessageRegistrationIntentTx(ctx, tx, message, reference); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) createMessageRegistrationIntentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	reference apiv1.EntityReference,
) error {
	snapshot, err := json.Marshal(reference)
	if err != nil {
		return fmt.Errorf("encode registration reference snapshot: %w", err)
	}
	parsedURL, err := url.Parse(reference.URL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.User != nil {
		return fmt.Errorf("registration reference URL is invalid")
	}
	providerHost := parsedURL.Scheme + "://" + parsedURL.Host
	if reference.Provider == messageRegistrationProviderGitHub {
		providerHost = "https://" + strings.ToLower(parsedURL.Hostname())
	}
	identity := r.captureMessageRegistrationIdentityTx(ctx, tx, message.TaskID, providerHost, reference)
	fingerprintData, err := messageRegistrationFingerprint(message, reference, snapshot)
	if err != nil {
		return err
	}
	actorID := strings.TrimSpace(message.AuthorID)
	if actorID == "" {
		actorID = userstore.DefaultUserID
	}
	intent := &TaskChangeRegistrationIntent{
		TaskID: message.TaskID, SourceKind: TaskChangeRegistrationSourceMessage,
		OriginID: message.ID, SourceRevision: strconv.Itoa(message.PromptIndex), ActorID: actorID,
		SelectionID: reference.SelectionID, ReferenceRef: reference.Ref, ReferenceSnapshot: string(snapshot),
		Provider:     reference.Provider,
		ProviderHost: firstNonEmptyRegistrationValue(identity.providerHost, providerHost), RepositoryID: identity.repositoryID,
		ConnectionScope: identity.connectionScope, ProviderRepositoryID: identity.providerRepositoryID,
		ChangeRequestID: strconv.FormatInt(identity.changeRequestID, 10), ExpectedRevision: identity.expectedRevision,
		OperationID:          registrationMessageOperationID(message.TaskID, message.ID, reference.SelectionID),
		OperationFingerprint: sha256Hex(fingerprintData), State: taskChangeRegistrationStatePending,
	}
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT workspace_id FROM tasks WHERE id = ?`), message.TaskID).Scan(&intent.WorkspaceID); err != nil {
		return fmt.Errorf("resolve registration workspace: %w", err)
	}
	if _, _, err := r.CreateTaskChangeRegistrationIntentTx(ctx, tx, intent); err != nil {
		return err
	}
	return nil
}

func messageRegistrationFingerprint(
	message *models.Message,
	reference apiv1.EntityReference,
	snapshot []byte,
) ([]byte, error) {
	fingerprint, err := json.Marshal(struct {
		TaskID        string `json:"task_id"`
		MessageID     string `json:"message_id"`
		PromptIndex   int    `json:"prompt_index"`
		SelectionID   string `json:"selection_id"`
		ReferenceHash string `json:"reference_hash"`
	}{message.TaskID, message.ID, message.PromptIndex, reference.SelectionID, sha256Hex(snapshot)})
	if err != nil {
		return nil, fmt.Errorf("encode registration fingerprint: %w", err)
	}
	return fingerprint, nil
}

type queuedRegistrationProjection struct {
	ID          string `db:"id"`
	SelectionID string `db:"selection_id"`
	State       string `db:"state"`
	ErrorCode   string `db:"error_code"`
	MessageID   string `db:"message_id"`
}

func (r *Repository) transferQueueRegistrationOriginsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	originIDs []string,
) (map[string]queuedRegistrationProjection, error) {
	transferred := make(map[string]queuedRegistrationProjection)
	if tx == nil || message == nil || message.ID == "" || len(originIDs) == 0 {
		return transferred, nil
	}
	query := `SELECT id, selection_id, state, error_code, message_id
		FROM task_change_registration_intents
		WHERE task_id = ? AND source_kind IN (?, ?) AND origin_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	for _, originID := range originIDs {
		if strings.TrimSpace(originID) == "" {
			continue
		}
		var rows []queuedRegistrationProjection
		if err := tx.SelectContext(ctx, &rows, tx.Rebind(query), message.TaskID,
			TaskChangeRegistrationSourceQueueAdmission, TaskChangeRegistrationSourceQueueEdit, originID); err != nil {
			return nil, fmt.Errorf("read queued registration origins for dispatch: %w", err)
		}
		for _, projection := range rows {
			if projection.MessageID != "" && projection.MessageID != message.ID {
				return nil, ErrTaskChangeRegistrationConflict
			}
			if _, err := tx.ExecContext(ctx, tx.Rebind(`
				UPDATE task_change_registration_intents SET message_id = ?, updated_at = ?
				WHERE task_id = ? AND id = ? AND (message_id = '' OR message_id = ?)`),
				message.ID, time.Now().UTC(), message.TaskID, projection.ID, message.ID); err != nil {
				return nil, fmt.Errorf("transfer queued registration origin: %w", err)
			}
			if projection.SelectionID != "" {
				projection.MessageID = message.ID
				transferred[projection.SelectionID] = projection
			}
		}
	}
	return transferred, nil
}

func (r *Repository) projectTransferredQueueReferencesTx(
	ctx context.Context,
	tx *sqlx.Tx,
	message *models.Message,
	references []apiv1.EntityReference,
	transferred map[string]queuedRegistrationProjection,
) (string, error) {
	if len(transferred) == 0 {
		return "", nil
	}
	for index := range references {
		projection, found := transferred[references[index].SelectionID]
		if !found || references[index].AssociationMode != apiv1.EntityReferenceModeLinkOnSend {
			continue
		}
		status, err := messageRegistrationStatus(projection.State)
		if err != nil {
			return "", err
		}
		references[index].RegistrationStatus = status
		references[index].RegistrationErrorCode = boundedRegistrationError(projection.ErrorCode)
	}
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return "", fmt.Errorf("encode dispatched message metadata: %w", err)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return "", fmt.Errorf("decode dispatched message metadata: %w", err)
	}
	metadata["entity_references"] = references
	metadataJSON, err = json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode dispatched registration references: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_session_messages SET metadata = ?, updated_at = ? WHERE id = ?`),
		string(metadataJSON), time.Now().UTC(), message.ID); err != nil {
		return "", fmt.Errorf("project queued registration state to transcript: %w", err)
	}
	message.Metadata = metadata
	return string(metadataJSON), nil
}

type messageRegistrationIdentity struct {
	repositoryID         string
	providerHost         string
	connectionScope      string
	providerRepositoryID string
	changeRequestID      int64
	expectedRevision     *int64
}

type messageRegistrationRepository struct {
	ID             string `db:"id"`
	WorkspaceID    string `db:"workspace_id"`
	Provider       string `db:"provider"`
	ProviderRepoID string `db:"provider_repo_id"`
	ProviderHost   string `db:"provider_host"`
	ProviderOwner  string `db:"provider_owner"`
	ProviderName   string `db:"provider_name"`
	RemoteURL      string `db:"remote_url"`
}

func prepareMessageRegistrationMetadata(message *models.Message, metadataJSON string) (string, error) {
	if message == nil || message.AuthorType != models.MessageAuthorUser || metadataJSON == "" {
		return metadataJSON, nil
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return "", fmt.Errorf("decode message metadata for registration status: %w", err)
	}
	rawReferences, exists := metadata["entity_references"]
	if !exists {
		return metadataJSON, nil
	}
	encoded, err := json.Marshal(rawReferences)
	if err != nil {
		return "", fmt.Errorf("encode message references for registration status: %w", err)
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(encoded, &references); err != nil {
		return "", fmt.Errorf("decode message references for registration status: %w", err)
	}
	changed := false
	for index := range references {
		if references[index].AssociationMode == apiv1.EntityReferenceModeLinkOnSend {
			references[index].RegistrationStatus = apiv1.EntityReferenceRegistrationPending
			references[index].RegistrationErrorCode = ""
			changed = true
		} else if references[index].RegistrationStatus != "" || references[index].RegistrationErrorCode != "" {
			references[index].RegistrationStatus = ""
			references[index].RegistrationErrorCode = ""
			changed = true
		}
	}
	if !changed {
		return metadataJSON, nil
	}
	metadata["entity_references"] = references
	encoded, err = json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode message metadata for registration status: %w", err)
	}
	message.Metadata = metadata
	return string(encoded), nil
}

// captureMessageRegistrationIdentityTx snapshots only an already attached
// repository and its association revision. Missing or unavailable provider
// identity remains nullable so a resolver outage cannot reject the message or
// give later recovery authority to restore an association that changed later.
func (r *Repository) captureMessageRegistrationIdentityTx(
	ctx context.Context, tx *sqlx.Tx, taskID, providerHost string, reference apiv1.EntityReference,
) messageRegistrationIdentity {
	parsedURL, err := url.Parse(reference.URL)
	if err != nil || parsedURL.Host == "" || parsedURL.RawQuery != "" || parsedURL.Fragment != "" ||
		strings.Contains(parsedURL.EscapedPath(), "%") {
		return messageRegistrationIdentity{}
	}
	project, changeID, ok := messageRegistrationURLIdentity(reference.Provider, reference.Kind, parsedURL)
	if !ok {
		return messageRegistrationIdentity{}
	}
	repository, ok := r.attachedRegistrationRepositoryTx(ctx, tx, taskID, reference.Provider, parsedURL, project)
	if !ok || strings.TrimSpace(repository.ProviderRepoID) == "" {
		return messageRegistrationIdentity{}
	}
	identity := messageRegistrationIdentity{
		repositoryID: repository.ID, providerHost: messageRegistrationCanonicalHost(reference.Provider, repository.ProviderHost, providerHost),
		connectionScope:      "builtin:" + repository.WorkspaceID,
		providerRepositoryID: strings.TrimSpace(repository.ProviderRepoID), changeRequestID: changeID,
	}
	query, args, ok := messageRegistrationAssociationRevisionQuery(reference.Provider, taskID, identity, changeID)
	if !ok {
		return messageRegistrationIdentity{}
	}
	var revision int64
	err = tx.GetContext(ctx, &revision, tx.Rebind(query), args...)
	if err == nil || err == sql.ErrNoRows {
		identity.expectedRevision = &revision
	}
	return identity
}

func (r *Repository) attachedRegistrationRepositoryTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, provider string,
	parsedURL *url.URL,
	project string,
) (messageRegistrationRepository, bool) {
	var repositories []messageRegistrationRepository
	if err := tx.SelectContext(ctx, &repositories, tx.Rebind(`
		SELECT r.id, r.workspace_id, COALESCE(r.provider, '') AS provider,
			COALESCE(r.provider_repo_id, '') AS provider_repo_id,
			COALESCE(r.provider_host, '') AS provider_host,
			COALESCE(r.provider_owner, '') AS provider_owner,
			COALESCE(r.provider_name, '') AS provider_name,
			COALESCE(r.remote_url, '') AS remote_url
		FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = ? AND r.deleted_at IS NULL`), taskID); err != nil {
		return messageRegistrationRepository{}, false
	}
	var match messageRegistrationRepository
	matches := 0
	for _, repository := range repositories {
		if messageRegistrationRepositoryMatches(provider, parsedURL, project, repository) {
			match = repository
			matches++
		}
	}
	if matches != 1 {
		return messageRegistrationRepository{}, false
	}
	return match, true
}

func messageRegistrationAssociationRevisionQuery(
	provider, taskID string,
	identity messageRegistrationIdentity,
	changeID int64,
) (string, []any, bool) {
	query := ""
	var args []any
	switch provider {
	case messageRegistrationProviderGitHub:
		query = `SELECT association_revision FROM github_task_prs
			WHERE task_id = ? AND provider_host = ? AND connection_scope = ?
			AND provider_repository_id = ? AND pr_number = ? LIMIT 1`
		args = []any{taskID, identity.providerHost, identity.connectionScope, identity.providerRepositoryID, changeID}
	case messageRegistrationProviderGitLab:
		query = `SELECT association_revision FROM gitlab_task_mrs
			WHERE task_id = ? AND lower(host) = lower(?) AND connection_scope = ?
			AND provider_repository_id = ? AND mr_iid = ? LIMIT 1`
		args = []any{taskID, identity.providerHost, identity.connectionScope, identity.providerRepositoryID, changeID}
	default:
		return "", nil, false
	}
	return query, args, true
}

func messageRegistrationURLIdentity(provider, kind string, value *url.URL) (string, int64, bool) {
	parts := strings.Split(strings.Trim(value.Path, "/"), "/")
	switch {
	case provider == messageRegistrationProviderGitHub && kind == "pull_request" && len(parts) == 4 && strings.EqualFold(parts[2], "pull"):
		changeID, err := strconv.ParseInt(parts[3], 10, 64)
		return strings.Join(parts[:2], "/"), changeID, err == nil && changeID > 0
	case provider == messageRegistrationProviderGitLab && kind == "merge_request":
		for index := 1; index+2 < len(parts); index++ {
			if parts[index] == "-" && strings.EqualFold(parts[index+1], "merge_requests") {
				changeID, err := strconv.ParseInt(parts[index+2], 10, 64)
				return strings.Join(parts[:index], "/"), changeID, index > 0 && err == nil && changeID > 0
			}
		}
	}
	return "", 0, false
}

func messageRegistrationRepositoryMatches(
	provider string, referenceURL *url.URL, project string, repository messageRegistrationRepository,
) bool {
	if !strings.EqualFold(strings.TrimSpace(repository.Provider), provider) {
		return false
	}
	repositoryHost := strings.TrimSpace(repository.ProviderHost)
	if repositoryHost != "" {
		if strings.Contains(repositoryHost, "://") {
			parsed, err := url.Parse(repositoryHost)
			if err != nil || parsed.Hostname() == "" {
				return false
			}
			repositoryHost = parsed.Hostname()
		}
		if !strings.EqualFold(strings.TrimSuffix(repositoryHost, "."), strings.TrimSuffix(referenceURL.Hostname(), ".")) {
			return false
		}
	}
	repositoryPath := strings.Trim(path.Join(repository.ProviderOwner, repository.ProviderName), "/")
	if repositoryPath == "" {
		remote, err := url.Parse(repository.RemoteURL)
		if err == nil {
			repositoryPath = strings.Trim(strings.TrimSuffix(strings.Trim(remote.Path, "/"), ".git"), "/")
		}
	}
	if provider == messageRegistrationProviderGitLab && repository.ProviderHost != "" {
		base, err := url.Parse(normalizeMessageRegistrationHost(repository.ProviderHost))
		if err == nil {
			contextRoot := strings.Trim(base.Path, "/")
			if contextRoot != "" && strings.HasPrefix(strings.ToLower(project), strings.ToLower(contextRoot)+"/") {
				project = strings.TrimPrefix(project, contextRoot+"/")
			}
		}
	}
	return repositoryPath != "" && strings.EqualFold(repositoryPath, project)
}

func messageRegistrationCanonicalHost(provider, repositoryHost, referenceHost string) string {
	if provider == messageRegistrationProviderGitHub {
		host := strings.TrimSpace(repositoryHost)
		if host == "" {
			host = referenceHost
		}
		parsed, err := url.Parse(normalizeMessageRegistrationHost(host))
		if err != nil || parsed.Hostname() == "" {
			return ""
		}
		return "https://" + strings.ToLower(parsed.Hostname())
	}
	if strings.TrimSpace(repositoryHost) != "" {
		return normalizeMessageRegistrationHost(repositoryHost)
	}
	return referenceHost
}

func normalizeMessageRegistrationHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if !strings.Contains(host, "://") {
		return "https://" + strings.TrimPrefix(host, "//")
	}
	return host
}

func firstNonEmptyRegistrationValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func registrationMessageOperationID(taskID, messageID, selectionID string) string {
	return "message:" + sha256Hex([]byte(taskID+"\x00"+messageID+"\x00"+selectionID))
}
