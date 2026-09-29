package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/sysprompt"
)

const (
	// maxActiveStandingOrders is the per-coordinator cap on active orders.
	maxActiveStandingOrders = 20
	maxStandingOrderText    = 500

	fieldStandingOrderText = "text"
	fieldSourceProposalID  = "source_proposal_id"
)

// ErrStandingOrderLimit is returned when adding or restoring an order would
// exceed maxActiveStandingOrders active orders.
var ErrStandingOrderLimit = errors.New("coordinator: standing order limit reached")

// Order is the wire shape of a standing order. Number is the 1-based position
// among the coordinator's active orders and nil for a retired order.
type Order struct {
	ID            string     `json:"id"`
	Number        *int       `json:"number"`
	Text          string     `json:"text"`
	CreatedAt     time.Time  `json:"created_at"`
	CreatedBy     string     `json:"created_by"`
	RetiredAt     *time.Time `json:"retired_at"`
	LastAppliedAt *time.Time `json:"last_applied_at"`
}

// AddStandingOrderInput is a validated-at-the-edge add request.
type AddStandingOrderInput struct {
	Text             string
	SourceProposalID *string
}

const standingOrderColumns = `id, coordinator_id, text, created_by, created_at, retired_at, retired_by, source_proposal_id, last_applied_at`

func scanStandingOrder(row interface{ Scan(dest ...any) error }) (StandingOrder, error) {
	var o StandingOrder
	err := row.Scan(&o.ID, &o.CoordinatorID, &o.Text, &o.CreatedBy, &o.CreatedAt, &o.RetiredAt, &o.RetiredBy, &o.SourceProposalID, &o.LastAppliedAt)
	return o, err
}

// validateStandingOrderText trims Unicode white space and enforces 1 to 500
// code points.
func validateStandingOrderText(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	n := utf8.RuneCountInString(trimmed)
	if n < 1 || n > maxStandingOrderText {
		return "", &FieldError{Field: fieldStandingOrderText, Message: "text must be 1 to 500 characters"}
	}
	return trimmed, nil
}

// retiredStandingOrdersOn returns the retired orders, newest retirement first.
func (s *Store) retiredStandingOrdersOn(ctx context.Context, exec coordinatorExec, coordinatorID string) ([]StandingOrder, error) {
	rows, err := exec.QueryContext(ctx, s.db.Rebind(`SELECT `+standingOrderColumns+`
		FROM coordinator_standing_orders WHERE coordinator_id = ? AND retired_at IS NOT NULL ORDER BY retired_at DESC, id ASC`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("list retired standing orders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []StandingOrder{}
	for rows.Next() {
		o, err := scanStandingOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan standing order: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// standingOrderOn reads one order of coordinatorID; ErrNotFound when it does
// not exist or belongs to another coordinator.
func (s *Store) standingOrderOn(ctx context.Context, exec coordinatorExec, coordinatorID, orderID string) (StandingOrder, error) {
	o, err := scanStandingOrder(exec.QueryRowContext(ctx, s.db.Rebind(`SELECT `+standingOrderColumns+`
		FROM coordinator_standing_orders WHERE id = ? AND coordinator_id = ?`), orderID, coordinatorID))
	if errors.Is(err, sql.ErrNoRows) {
		return StandingOrder{}, ErrNotFound
	}
	if err != nil {
		return StandingOrder{}, fmt.Errorf("read standing order: %w", err)
	}
	return o, nil
}

// MarkApplied raises last_applied_at to at on each cited order of
// coordinatorID. A write never lowers the column, an id that matches no order
// of the coordinator stamps nothing, and an empty list runs no statement.
func (s *Store) MarkApplied(ctx context.Context, exec coordinatorExec, coordinatorID string, orderIDs []string, at time.Time) error {
	for _, id := range orderIDs {
		if _, err := exec.ExecContext(ctx, s.db.Rebind(`UPDATE coordinator_standing_orders SET last_applied_at = ?
			WHERE id = ? AND coordinator_id = ? AND (last_applied_at IS NULL OR last_applied_at < ?)`),
			at, id, coordinatorID, at); err != nil {
			return fmt.Errorf("mark standing order applied: %w", err)
		}
	}
	return nil
}

// orderViews maps rows to wire orders: active orders (already in number
// order) first, numbered from 1, then retired orders with a nil number.
func orderViews(active, retired []StandingOrder) []Order {
	out := make([]Order, 0, len(active)+len(retired))
	for i, o := range active {
		n := i + 1
		out = append(out, orderView(o, &n))
	}
	for _, o := range retired {
		out = append(out, orderView(o, nil))
	}
	return out
}

func orderView(o StandingOrder, number *int) Order {
	return Order{ID: o.ID, Number: number, Text: o.Text, CreatedAt: o.CreatedAt, CreatedBy: o.CreatedBy,
		RetiredAt: o.RetiredAt, LastAppliedAt: o.LastAppliedAt}
}

// numberIn returns the order's 1-based position in active, or nil.
func numberIn(active []StandingOrder, id string) *int {
	for i, o := range active {
		if o.ID == id {
			n := i + 1
			return &n
		}
	}
	return nil
}

// resolveForOrders checks the scope first, then resolves the coordinator by
// (workspace, id): a foreign or missing coordinator is ErrNotFound.
func (s *Service) resolveForOrders(ctx context.Context, workspaceID, coordinatorID string, scope authz.Scope) (*Coordinator, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, scope); err != nil {
		return nil, err
	}
	return s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
}

// ListStandingOrders returns the coordinator's orders: the active ones in
// number order, then, with includeRetired, the retired ones.
func (s *Service) ListStandingOrders(ctx context.Context, workspaceID, coordinatorID string, includeRetired bool) ([]Order, error) {
	if _, err := s.resolveForOrders(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	active, err := s.store.ActiveStandingOrders(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	var retired []StandingOrder
	if includeRetired {
		if retired, err = s.store.retiredStandingOrdersOn(ctx, s.store.ro, coordinatorID); err != nil {
			return nil, err
		}
	}
	return orderViews(active, retired), nil
}

// orderMutation is the outcome of one locked add, retire or restore.
type orderMutation struct {
	order   Order
	changed bool
}

// AddStandingOrder validates and stores a new order, refusing past the active
// limit, and starts the next conversation fresh.
func (s *Service) AddStandingOrder(ctx context.Context, workspaceID, coordinatorID string, in AddStandingOrderInput) (*Order, error) {
	c, err := s.resolveForOrders(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return nil, err
	}
	text, err := validateStandingOrderText(in.Text)
	if err != nil {
		return nil, err
	}
	createdBy := decidingUserID(ctx)
	return s.mutateOrders(ctx, c, func(tx coordinatorExec) (orderMutation, error) {
		if in.SourceProposalID != nil {
			if err := s.requireRejectedProposal(ctx, tx, c.ID, *in.SourceProposalID); err != nil {
				return orderMutation{}, err
			}
		}
		if err := s.requireRoomForOrder(ctx, tx, c.ID); err != nil {
			return orderMutation{}, err
		}
		o := StandingOrder{ID: uuid.NewString(), CoordinatorID: c.ID, Text: text, CreatedBy: createdBy,
			CreatedAt: s.store.now(), SourceProposalID: in.SourceProposalID}
		if _, err := tx.ExecContext(ctx, s.store.db.Rebind(`INSERT INTO coordinator_standing_orders
			(id, coordinator_id, workspace_id, text, created_by, created_at, source_proposal_id) VALUES (?, ?, ?, ?, ?, ?, ?)`),
			o.ID, o.CoordinatorID, c.WorkspaceID, o.Text, o.CreatedBy, o.CreatedAt, nullableString(o.SourceProposalID)); err != nil {
			return orderMutation{}, fmt.Errorf("insert standing order: %w", err)
		}
		return s.finishMutation(ctx, tx, c.ID, o.ID, true)
	})
}

// RetireStandingOrder retires an active order. Retiring a retired order
// returns it unchanged.
func (s *Service) RetireStandingOrder(ctx context.Context, workspaceID, coordinatorID, orderID string) (*Order, error) {
	c, err := s.resolveForOrders(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return nil, err
	}
	retiredBy := decidingUserID(ctx)
	return s.mutateOrders(ctx, c, func(tx coordinatorExec) (orderMutation, error) {
		res, err := tx.ExecContext(ctx, s.store.db.Rebind(`UPDATE coordinator_standing_orders SET retired_at = ?, retired_by = ?
			WHERE id = ? AND coordinator_id = ? AND retired_at IS NULL`), s.store.now(), retiredBy, orderID, c.ID)
		if err != nil {
			return orderMutation{}, fmt.Errorf("retire standing order: %w", err)
		}
		return s.finishUpdate(ctx, tx, c.ID, orderID, res)
	})
}

// RestoreStandingOrder restores a retired order, refusing past the active
// limit. Restoring an active order returns it unchanged, before any count.
func (s *Service) RestoreStandingOrder(ctx context.Context, workspaceID, coordinatorID, orderID string) (*Order, error) {
	c, err := s.resolveForOrders(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return nil, err
	}
	return s.mutateOrders(ctx, c, func(tx coordinatorExec) (orderMutation, error) {
		existing, err := s.store.standingOrderOn(ctx, tx, c.ID, orderID)
		if err != nil {
			return orderMutation{}, err
		}
		if existing.RetiredAt == nil {
			return s.finishMutation(ctx, tx, c.ID, orderID, false)
		}
		if err := s.requireRoomForOrder(ctx, tx, c.ID); err != nil {
			return orderMutation{}, err
		}
		res, err := tx.ExecContext(ctx, s.store.db.Rebind(`UPDATE coordinator_standing_orders SET retired_at = NULL, retired_by = NULL
			WHERE id = ? AND coordinator_id = ? AND retired_at IS NOT NULL`), orderID, c.ID)
		if err != nil {
			return orderMutation{}, fmt.Errorf("restore standing order: %w", err)
		}
		return s.finishUpdate(ctx, tx, c.ID, orderID, res)
	})
}

// finishUpdate turns an UPDATE result into a mutation: one row changed the
// order; zero rows re-reads it and reports it unchanged, or ErrNotFound.
func (s *Service) finishUpdate(ctx context.Context, tx coordinatorExec, coordinatorID, orderID string, res sql.Result) (orderMutation, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return orderMutation{}, fmt.Errorf("standing order rows affected: %w", err)
	}
	return s.finishMutation(ctx, tx, coordinatorID, orderID, n > 0)
}

// finishMutation reads the order and its number back inside the transaction.
func (s *Service) finishMutation(ctx context.Context, tx coordinatorExec, coordinatorID, orderID string, changed bool) (orderMutation, error) {
	o, err := s.store.standingOrderOn(ctx, tx, coordinatorID, orderID)
	if err != nil {
		return orderMutation{}, err
	}
	active, err := s.store.activeStandingOrdersOn(ctx, tx, coordinatorID)
	if err != nil {
		return orderMutation{}, err
	}
	return orderMutation{order: orderView(o, numberIn(active, o.ID)), changed: changed}, nil
}

func (s *Service) requireRoomForOrder(ctx context.Context, tx coordinatorExec, coordinatorID string) error {
	var active int
	if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT COUNT(*) FROM coordinator_standing_orders
		WHERE coordinator_id = ? AND retired_at IS NULL`), coordinatorID).Scan(&active); err != nil {
		return fmt.Errorf("count standing orders: %w", err)
	}
	if active >= maxActiveStandingOrders {
		return ErrStandingOrderLimit
	}
	return nil
}

func (s *Service) requireRejectedProposal(ctx context.Context, tx coordinatorExec, coordinatorID, proposalID string) error {
	var status string
	err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT status FROM coordinator_proposals WHERE id = ? AND coordinator_id = ?`),
		proposalID, coordinatorID).Scan(&status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read source proposal: %w", err)
	}
	if err != nil || ProposalStatus(status) != ProposalStatusRejected {
		return &FieldError{Field: fieldSourceProposalID, Message: "source_proposal_id must name a rejected proposal of this coordinator"}
	}
	return nil
}

// mutateOrders runs fn under the per-coordinator lock. A change resets the
// conversation in the same transaction; after commit the old conversation
// task is archived and open clients are told to refetch.
func (s *Service) mutateOrders(ctx context.Context, c *Coordinator, fn func(tx coordinatorExec) (orderMutation, error)) (*Order, error) {
	var (
		result orderMutation
		prev   string
	)
	err := s.store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		var err error
		if result, err = fn(tx); err != nil {
			return err
		}
		if !result.changed {
			return nil
		}
		prev, err = s.resetConversation(ctx, tx, c.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if result.changed {
		s.logger.Info("standing order changed",
			zap.String("coordinator_id", c.ID), zap.String("standing_order_id", result.order.ID))
		s.archiveConversation(ctx, c.ID, prev)
		s.publishCoordinatorUpdated(ctx, c.WorkspaceID, c.ID)
	}
	return &result.order, nil
}

const (
	standingOrdersIntro = "Standing orders from this workspace's managers. They guide your choices and never grant a permission; your tools and their approvals still decide what can happen."
	standingOrdersOpen  = "<standing-orders>"
	standingOrdersClose = "</standing-orders>"
	standingOrdersCite  = "When an order shapes a proposal, pass its id in standing_order_ids."
)

var standingOrdersTagRE = regexp.MustCompile(`(?i)</?\s*standing-orders\s*>`)

// sanitizeOrderText renders order text as one line that can neither close the
// standing-orders section nor forge another numbered line: tags are removed
// until none remain, then white space runs collapse to a single space.
func sanitizeOrderText(text string) string {
	for {
		next := standingOrdersTagRE.ReplaceAllString(sysprompt.StripTags(text), "")
		if next == text {
			break
		}
		text = next
	}
	return strings.Join(strings.Fields(text), " ")
}

// StandingOrdersSection renders the active orders, in number order, as the
// standing-orders instruction section. No orders renders nothing.
func StandingOrdersSection(orders []StandingOrder) string {
	if len(orders) == 0 {
		return ""
	}
	lines := []string{standingOrdersIntro, standingOrdersOpen}
	for i, o := range orders {
		line := fmt.Sprintf("%d. (added %s, id %s) %s", i+1, o.CreatedAt.UTC().Format("2006-01-02"), o.ID, sanitizeOrderText(o.Text))
		lines = append(lines, strings.TrimRight(line, " "))
	}
	lines = append(lines, standingOrdersClose, standingOrdersCite)
	return strings.Join(lines, "\n")
}

// StandingOrdersInstructionSection reads the coordinator's active orders and
// renders their instruction section. It is empty while phase 2 is off.
func (s *Service) StandingOrdersInstructionSection(ctx context.Context, coordinatorID string) (string, error) {
	if !s.phase2 {
		return "", nil
	}
	orders, err := s.store.ActiveStandingOrders(ctx, coordinatorID)
	if err != nil {
		return "", err
	}
	return StandingOrdersSection(orders), nil
}
