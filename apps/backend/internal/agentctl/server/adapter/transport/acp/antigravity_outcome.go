package acp

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	antigravityJournalBusyTimeoutMS = 100
	antigravityJournalQueryTimeout  = 2 * time.Second
	antigravityNativeUnavailable    = "native journal is unavailable"
	antigravityNativeDisappeared    = "native journal disappeared"
	antigravityNativeReplaced       = "native journal file was replaced"
	antigravityNativeRootUnusable   = "native journal root is unavailable"
	antigravityNativeReadFailed     = "native journal could not be read"
	antigravityNativeOpenFailed     = "native journal could not be opened"
	antigravityNativeSchema         = "native journal schema is not recognized"
	antigravityRefusalStopReason    = "refusal"
	antigravityCancelledStopReason  = "cancelled"
	antigravitySQLiteTextType       = "text"
)

// antigravityOutcomeState describes how far the bounded native journal probe
// could verify the active session's store.
type antigravityOutcomeState uint8

const (
	// antigravityOutcomeNoStore means the native config root does not exist.
	// No failure is invented for an absent store.
	antigravityOutcomeNoStore antigravityOutcomeState = iota
	// antigravityOutcomeUnavailable means the store exists but its evidence
	// could not be verified (link, replacement, schema, or read failure).
	antigravityOutcomeUnavailable
	// antigravityOutcomeAbsent means the store root exists but the active
	// session's database is not present yet.
	antigravityOutcomeAbsent
	// antigravityOutcomePresent means the session database passed every
	// containment, type, and identity check.
	antigravityOutcomePresent
)

type antigravityJournalStore struct {
	state      antigravityOutcomeState
	root       string
	dbPath     string
	startIndex int64
	identity   os.FileInfo
	errorStep  *antigravityErrorStep
	reason     string
}

type antigravityErrorStep struct {
	idx     int64
	payload []byte
}

type antigravityOutcomeBoundary struct {
	sessionID string
	store     antigravityJournalStore
}

type antigravityNativeFailure struct {
	code          routingerr.Code
	message       string
	providerError *streams.ProviderError
}

// captureAntigravityOutcomeBoundary records the active native session's
// high-water index before a prompt. A nil boundary means the agent is not
// Antigravity or the session identity is unusable.
func (a *Adapter) captureAntigravityOutcomeBoundary(sessionID string) *antigravityOutcomeBoundary {
	if a == nil || !acpcompat.IsAntigravity(a.agentID) {
		return nil
	}
	if !antigravityValidSessionID(sessionID) {
		return &antigravityOutcomeBoundary{
			sessionID: sessionID,
			store: antigravityJournalStore{
				state:  antigravityOutcomeUnavailable,
				reason: "native session identity is not verifiable",
			},
		}
	}
	base, ok := a.antigravityJournalBase()
	if !ok {
		return &antigravityOutcomeBoundary{
			sessionID: sessionID,
			store: antigravityJournalStore{
				state:  antigravityOutcomeUnavailable,
				reason: antigravityNativeRootUnusable,
			},
		}
	}
	return &antigravityOutcomeBoundary{
		sessionID: sessionID,
		store:     inspectAntigravityJournal(a.outcomeContext(), base, sessionID, nil),
	}
}

// checkAntigravityOutcome inspects only records appended after the boundary.
// A non-nil failure means the turn failed. A non-empty reason means the turn
// could not be verified and success must be withheld.
func (a *Adapter) checkAntigravityOutcome(
	boundary *antigravityOutcomeBoundary,
	stopReason string,
) (*antigravityNativeFailure, string) {
	if boundary == nil || !a.isActiveSession(boundary.sessionID) {
		return nil, ""
	}
	if stopReason == antigravityCancelledStopReason {
		return nil, ""
	}
	if a.antigravityQuotaRefusal(stopReason) {
		return a.antigravityRefusalFailure(), ""
	}
	if boundary.store.state == antigravityOutcomeNoStore {
		start := int64(-1)
		after := inspectAntigravityJournal(
			a.outcomeContext(), boundary.store.root, boundary.sessionID, &start,
		)
		if after.state == antigravityOutcomeNoStore {
			return nil, ""
		}
		if after.state == antigravityOutcomeUnavailable {
			return nil, after.reason
		}
		if after.state != antigravityOutcomePresent {
			return nil, antigravityNativeUnavailable
		}
		if after.errorStep == nil {
			return nil, ""
		}
		return a.antigravityFailureFromErrorStep(after.errorStep), ""
	}
	if boundary.store.state == antigravityOutcomeUnavailable {
		return nil, boundary.store.reason
	}
	after := inspectAntigravityJournal(
		a.outcomeContext(), boundary.store.root, boundary.sessionID, &boundary.store.startIndex,
	)
	return a.antigravityOutcomeAfter(boundary, after)
}

func (a *Adapter) antigravityOutcomeAfter(
	boundary *antigravityOutcomeBoundary,
	after antigravityJournalStore,
) (*antigravityNativeFailure, string) {
	if after.state == antigravityOutcomeUnavailable {
		return nil, after.reason
	}
	if after.state == antigravityOutcomeNoStore {
		if boundary.store.state == antigravityOutcomeNoStore {
			return nil, ""
		}
		return nil, antigravityNativeDisappeared
	}
	if after.state == antigravityOutcomeAbsent {
		return nil, antigravityNativeUnavailable
	}
	if boundary.store.state == antigravityOutcomePresent &&
		!antigravitySameJournalIdentity(boundary.store.identity, after.identity) {
		return nil, antigravityNativeReplaced
	}
	if after.errorStep == nil {
		return nil, ""
	}
	return a.antigravityFailureFromErrorStep(after.errorStep), ""
}

func antigravitySameJournalIdentity(before, after os.FileInfo) bool {
	return before != nil && after != nil && os.SameFile(before, after)
}

// antigravityPromptProviderError normalizes a peer-returned ACP prompt error
// for Antigravity. It never treats a generic -32000 as authentication
// evidence and preserves the connection for a retained failure.
func (a *Adapter) antigravityPromptProviderError(err error) (*streams.ProviderError, bool) {
	if a == nil || !acpcompat.IsAntigravity(a.agentID) {
		return nil, false
	}
	requestErr, ok := remotePromptRequestError(err)
	if !ok {
		return nil, false
	}
	env := acpcompat.ParseAntigravityACPMessage(requestErr.Code, requestErr.Message, requestErr.Data)
	failure := routingerr.ClassifyAntigravity(a.antigravityRoutingInput(env))
	normalized := a.antigravityFailure(streams.ProviderErrorSourceAntigravityACP, failure, env.Message)
	normalized.providerError.RPCCode = requestErr.Code
	return normalized.providerError, true
}

func (a *Adapter) antigravityRoutingInput(env acpcompat.AntigravityErrorEnvelope) routingerr.AntigravityFailure {
	return routingerr.AntigravityFailure{
		RPCCode:           env.RPCCode,
		AuthMethodsCached: a.antigravityAuthMethodsCached(),
		HTTPStatus:        env.HTTPStatus,
		Status:            env.Status,
		Reason:            env.Reason,
		Message:           env.Message,
		QuotaExhausted:    env.QuotaExhausted,
		ResetHint:         env.ResetHint,
		ResetDelay:        env.ResetDelay,
		OccurredAt:        time.Now().UTC(),
		Phase:             routingerr.PhasePromptSend,
	}
}

func (a *Adapter) antigravityAuthMethodsCached() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.availableAuthMethods) > 0
}

func (a *Adapter) antigravityQuotaRefusal(stopReason string) bool {
	if stopReason != antigravityRefusalStopReason {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentInfo != nil && a.agentInfo.Version == acpcompat.AntigravityQuotaRefusalVersion
}

func (a *Adapter) antigravityRefusalFailure() *antigravityNativeFailure {
	failure := routingerr.ClassifyAntigravity(routingerr.AntigravityFailure{
		QuotaRefusal: true,
		OccurredAt:   time.Now().UTC(),
		Phase:        routingerr.PhasePromptSend,
	})
	return a.antigravityFailure(streams.ProviderErrorSourceAntigravityNative, failure, "")
}

func (a *Adapter) antigravityFailureFromErrorStep(step *antigravityErrorStep) *antigravityNativeFailure {
	env, _ := acpcompat.ParseAntigravityNativeErrorPayload(step.payload)
	failure := routingerr.ClassifyAntigravity(a.antigravityRoutingInput(env))
	return a.antigravityFailure(streams.ProviderErrorSourceAntigravityNative, failure, env.Message)
}

func (a *Adapter) antigravityFailure(
	source string,
	failure *routingerr.Error,
	detail string,
) *antigravityNativeFailure {
	detail = streams.SanitizeProviderMessage(detail)
	message := streams.SanitizeProviderMessage(routingerr.AntigravityFailureMessage(failure.Code, detail))
	if message == "" {
		message = streams.SanitizeProviderMessage(routingerr.AntigravityUnknownMessage)
	}
	providerID, modelID := a.ProviderErrorContext()
	return &antigravityNativeFailure{
		code:    failure.Code,
		message: message,
		providerError: &streams.ProviderError{
			Source:     source,
			ProviderID: providerID,
			ModelID:    modelID,
			Message:    message,
			OccurredAt: time.Now().UTC(),
			ResetAt:    failure.ResetHint,
		},
	}
}

func (a *Adapter) antigravityOutcomeUnverifiedFailure(reason string) *antigravityNativeFailure {
	message := routingerr.AntigravityOutcomeUnverifiedMessage
	if detail := strings.TrimSpace(reason); detail != "" {
		message += ": " + detail
	}
	message = streams.SanitizeProviderMessage(message)
	providerID, modelID := a.ProviderErrorContext()
	return &antigravityNativeFailure{
		code:    routingerr.CodeAgentRuntime,
		message: message,
		providerError: &streams.ProviderError{
			Source:     streams.ProviderErrorSourceAntigravityNative,
			ProviderID: providerID,
			ModelID:    modelID,
			Message:    message,
			OccurredAt: time.Now().UTC(),
		},
	}
}

func (a *Adapter) antigravityJournalBase() (string, bool) {
	if a.antigravityJournalRoot != "" {
		return a.antigravityJournalRoot, true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, acpcompat.AntigravityConfigRelDir), true
}

func (a *Adapter) outcomeContext() context.Context {
	if a.lifetimeCtx != nil {
		return a.lifetimeCtx
	}
	return context.Background()
}

func antigravityValidSessionID(sessionID string) bool {
	if sessionID == "" || len(sessionID) > 200 {
		return false
	}
	for _, r := range sessionID {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// isAntigravityNonModelCommand reports the one validated native command whose
// RPC result is authoritative without a model trajectory row. Other
// slash-prefixed prompts stay subject to the outcome check.
func isAntigravityNonModelCommand(message string) bool {
	return strings.TrimSpace(message) == "/usage"
}

func inspectAntigravityJournal(
	ctx context.Context,
	base string,
	sessionID string,
	after *int64,
) antigravityJournalStore {
	store := resolveAntigravityJournalPaths(base, sessionID)
	if store.state != antigravityOutcomePresent {
		return store
	}
	queryCtx, cancel := context.WithTimeout(ctx, antigravityJournalQueryTimeout)
	defer cancel()
	database, err := openAntigravityJournal(store.dbPath)
	if err != nil {
		return antigravityStoreFailure(store, antigravityNativeOpenFailed)
	}
	defer func() { _ = database.Close() }()
	if err := validateAntigravityStepsSchema(queryCtx, database); err != nil {
		return antigravityStoreFailure(store, antigravityNativeSchema)
	}
	if after == nil {
		index, err := queryAntigravityMaxIdx(queryCtx, database)
		if err != nil {
			return antigravityStoreFailure(store, antigravityNativeReadFailed)
		}
		store.startIndex = index
		return store
	}
	step, err := queryAntigravityErrorStep(queryCtx, database, *after)
	if err != nil {
		return antigravityStoreFailure(store, antigravityNativeReadFailed)
	}
	store.errorStep = step
	return store
}

func resolveAntigravityJournalPaths(base, sessionID string) antigravityJournalStore {
	store := antigravityJournalStore{state: antigravityOutcomeNoStore, root: base}
	rootInfo, err := os.Lstat(base)
	if os.IsNotExist(err) {
		return store
	}
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return antigravityStoreFailure(store, antigravityNativeRootUnusable)
	}
	realRoot, err := filepath.EvalSymlinks(base)
	if err != nil {
		return antigravityStoreFailure(store, antigravityNativeRootUnusable)
	}
	store.root = realRoot
	conversations := filepath.Join(realRoot, acpcompat.AntigravityConversationsDir)
	if !antigravityPathWithin(realRoot, conversations) {
		return antigravityStoreFailure(store, antigravityNativeRootUnusable)
	}
	conversationsInfo, err := os.Lstat(conversations)
	if os.IsNotExist(err) {
		store.state = antigravityOutcomeAbsent
		store.startIndex = -1
		return store
	}
	if err != nil || conversationsInfo.Mode()&os.ModeSymlink != 0 || !conversationsInfo.IsDir() {
		return antigravityStoreFailure(store, antigravityNativeUnavailable)
	}
	realConversations, err := filepath.EvalSymlinks(conversations)
	if err != nil || realConversations != conversations {
		return antigravityStoreFailure(store, antigravityNativeUnavailable)
	}
	return resolveAntigravityJournalFile(store, conversations, sessionID)
}

func resolveAntigravityJournalFile(
	store antigravityJournalStore,
	conversations string,
	sessionID string,
) antigravityJournalStore {
	dbPath := filepath.Join(conversations, sessionID+".db")
	store.dbPath = dbPath
	if !antigravityPathWithin(conversations, dbPath) {
		return antigravityStoreFailure(store, antigravityNativeUnavailable)
	}
	dbInfo, err := os.Lstat(dbPath)
	if os.IsNotExist(err) {
		store.state = antigravityOutcomeAbsent
		store.startIndex = -1
		return store
	}
	if err != nil || dbInfo.Mode()&os.ModeSymlink != 0 || !dbInfo.Mode().IsRegular() {
		return antigravityStoreFailure(store, antigravityNativeUnavailable)
	}
	realDB, err := filepath.EvalSymlinks(dbPath)
	if err != nil || realDB != dbPath {
		return antigravityStoreFailure(store, antigravityNativeUnavailable)
	}
	statInfo, err := os.Stat(dbPath)
	if err != nil || !os.SameFile(dbInfo, statInfo) {
		return antigravityStoreFailure(store, antigravityNativeReplaced)
	}
	store.state = antigravityOutcomePresent
	store.identity = statInfo
	store.startIndex = -1
	return store
}

func antigravityStoreFailure(store antigravityJournalStore, reason string) antigravityJournalStore {
	store.state = antigravityOutcomeUnavailable
	store.reason = reason
	return store
}

func antigravityPathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func openAntigravityJournal(path string) (*sql.DB, error) {
	// mode=ro keeps WAL visibility without taking a write lock; the short
	// busy timeout bounds a locked read, and query_only rejects accidental
	// writes to the vendor store.
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := "file:" + escapeAntigravitySQLitePath(uriPath) +
		fmt.Sprintf("?mode=ro&_query_only=1&_busy_timeout=%d", antigravityJournalBusyTimeoutMS)
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	return database, nil
}

var antigravitySQLitePathReplacer = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23")

func escapeAntigravitySQLitePath(path string) string {
	return antigravitySQLitePathReplacer.Replace(path)
}

func validateAntigravityStepsSchema(ctx context.Context, database *sql.DB) error {
	var name string
	err := database.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'steps'`,
	).Scan(&name)
	if err != nil {
		return fmt.Errorf("read steps table: %w", err)
	}
	rows, err := database.QueryContext(ctx, `PRAGMA table_info(steps)`)
	if err != nil {
		return fmt.Errorf("read steps schema: %w", err)
	}
	defer func() { _ = rows.Close() }()
	columns := make(map[string]struct{}, 3)
	for rows.Next() {
		var (
			cid       int
			column    string
			columnTyp string
			notNull   int
			defaultV  sql.NullString
			primary   int
		)
		if err := rows.Scan(&cid, &column, &columnTyp, &notNull, &defaultV, &primary); err != nil {
			return fmt.Errorf("scan steps schema: %w", err)
		}
		columns[strings.ToLower(column)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read steps schema: %w", err)
	}
	for _, required := range []string{"idx", "step_type", "step_payload"} {
		if _, ok := columns[required]; !ok {
			return fmt.Errorf("steps schema is missing %s", required)
		}
	}
	return validateAntigravityStepsTypes(ctx, database)
}

func validateAntigravityStepsTypes(ctx context.Context, database *sql.DB) error {
	var idxType, stepType, payloadType string
	err := database.QueryRowContext(
		ctx,
		`SELECT typeof(idx), typeof(step_type), typeof(step_payload) FROM steps LIMIT 1`,
	).Scan(&idxType, &stepType, &payloadType)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read steps types: %w", err)
	}
	if !antigravityNumericSQLiteType(idxType) || !antigravityNumericSQLiteType(stepType) {
		return fmt.Errorf("steps idx/step_type are not numeric")
	}
	if payloadType != "blob" && payloadType != antigravitySQLiteTextType && payloadType != "null" {
		return fmt.Errorf("steps step_payload is not text or blob")
	}
	return nil
}

func antigravityNumericSQLiteType(value string) bool {
	return value == "integer" || value == "real"
}

func queryAntigravityMaxIdx(ctx context.Context, database *sql.DB) (int64, error) {
	var index sql.NullInt64
	if err := database.QueryRowContext(ctx, `SELECT MAX(idx) FROM steps`).Scan(&index); err != nil {
		return 0, err
	}
	if !index.Valid {
		return -1, nil
	}
	return index.Int64, nil
}

func queryAntigravityErrorStep(
	ctx context.Context,
	database *sql.DB,
	after int64,
) (*antigravityErrorStep, error) {
	var (
		step    antigravityErrorStep
		payload []byte
	)
	err := database.QueryRowContext(
		ctx,
		`SELECT CAST(idx AS INTEGER), substr(step_payload, 1, ?)
		 FROM steps
		 WHERE CAST(step_type AS INTEGER) = ? AND CAST(idx AS INTEGER) > ?
		 ORDER BY CAST(idx AS INTEGER) DESC
		 LIMIT 1`,
		acpcompat.AntigravityMaxErrorPayloadBytes,
		acpcompat.AntigravityErrorStepType,
		after,
	).Scan(&step.idx, &payload)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	step.payload = payload
	return &step, nil
}
