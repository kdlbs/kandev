package webapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultEventCapacity              = 1000
	DefaultEventReplayWindow          = 5 * time.Minute
	DefaultEventHistoryTTL            = DefaultEventReplayWindow
	DefaultEventHeartbeatInterval     = 15 * time.Second
	DefaultEventSubscriberQueueSize   = 32
	DefaultEventMaxStreamsPerUser     = 20
	DefaultEventMaxStreamsPerInstance = 2
	DefaultEventMaxPayloadBytes       = 256 << 10
	DefaultEventStreamLifetime        = 15 * time.Minute

	RuntimeResyncRequired = "runtime.resync_required"

	ResyncReasonHistoryGap         = "history_gap"
	ResyncReasonGenerationMismatch = "generation_mismatch"
	ResyncReasonInvalidCursor      = "invalid_cursor"
	ResyncReasonCursorAhead        = "cursor_ahead"
)

var (
	ErrEventHubClosed       = errors.New("webapp: event hub is closed")
	ErrInvalidEvent         = errors.New("webapp: invalid event")
	ErrEventPayloadTooLarge = errors.New("webapp: event payload exceeds limit")
	ErrInvalidSubscription  = errors.New("webapp: invalid event subscription")
	ErrTooManyEventStreams  = errors.New("webapp: too many event streams")
	ErrSlowSubscriber       = errors.New("webapp: slow event subscriber")
)

// EventScope identifies the resources represented by an event. InstanceID is
// always populated by EventHub; the remaining fields are optional and are
// retained as public scope metadata for the web application.
type EventScope struct {
	InstanceID   string `json:"instance_id"`
	WorkspaceID  string `json:"workspace_id,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	RepositoryID string `json:"repository_id,omitempty"`
}

// EventInput is the public, bounded input accepted by Publish. Data is
// marshaled before the event is retained, so callers cannot mutate history by
// changing a map or slice after Publish returns.
type EventInput struct {
	Type  string
	Scope EventScope
	Data  any
}

// Event is the immutable public event delivered to subscribers and encoded in
// an SSE data field. ID is generation:sequence and is stable for replay.
type Event struct {
	ID         string          `json:"id,omitempty"`
	Generation string          `json:"generation"`
	Sequence   uint64          `json:"sequence,omitempty"`
	Type       string          `json:"type"`
	Scope      EventScope      `json:"scope"`
	Data       json.RawMessage `json:"data"`
	published  time.Time       `json:"-"`
}

// EventFilter is evaluated for every live and replayed event. A parent
// runtime can close over current grants and scope checks so authorization is
// re-evaluated without making the hub depend on the Service layer.
type EventFilter func(Event) bool

// ResyncInfo is the payload of RuntimeResyncRequired. The empty SSE id sent
// with this event resets the browser's Last-Event-ID cursor.
type ResyncInfo struct {
	Reason     string `json:"reason"`
	Generation string `json:"generation"`
	Reset      bool   `json:"reset"`
}

// EventSubscriptionRequest binds a stream to one authorized user and plugin
// instance. The request is intended to be created by the capability runtime,
// not from browser-supplied scope fields.
type EventSubscriptionRequest struct {
	InstanceID  string
	UserID      string
	LastEventID string
	Filter      EventFilter
}

// EventHubConfig provides bounded operational limits. Zero values use the
// production defaults. Now is injectable for deterministic replay expiry
// tests.
type EventHubConfig struct {
	Generation                string
	MaxEvents                 int
	ReplayWindow              time.Duration
	SubscriberQueueSize       int
	HeartbeatInterval         time.Duration
	StreamLifetime            time.Duration
	MaxStreamsPerUser         int
	MaxStreamsPerUserInstance int
	MaxPayloadBytes           int
	Now                       func() time.Time
}

// EventHub is an in-memory, per-process event transport. Event history is
// partitioned by plugin instance, while the generation identifies this
// process lifetime for reconnect recovery.
type EventHub struct {
	mu                  sync.Mutex
	instances           map[string]*eventInstance
	userStreams         map[string]int
	userInstanceStreams map[eventStreamKey]int
	closed              bool
	generation          string
	config              EventHubConfig
	now                 func() time.Time
}

type eventInstance struct {
	lastSequence uint64
	events       []Event
	subscribers  map[*EventSubscription]struct{}
}

type eventStreamKey struct {
	userID     string
	instanceID string
}

// EventSubscription owns one bounded subscriber queue. Closing the
// subscription is idempotent and safe to race with Publish, Close, and
// request cancellation.
type EventSubscription struct {
	hub        *EventHub
	instanceID string
	userID     string
	filter     EventFilter
	events     chan Event
	done       chan struct{}

	stateMu sync.RWMutex
	closed  bool
	err     error
}

// NewEventHub creates a hub with production defaults. At most one config is
// used; the variadic form permits both NewEventHub() and concise test setup.
func NewEventHub(configs ...EventHubConfig) *EventHub {
	config := EventHubConfig{}
	if len(configs) > 0 {
		config = configs[0]
	}
	applyEventHubDefaults(&config)
	generation := strings.TrimSpace(config.Generation)
	if !validGeneration(generation) {
		generation = newEventGeneration()
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &EventHub{
		instances:           make(map[string]*eventInstance),
		userStreams:         make(map[string]int),
		userInstanceStreams: make(map[eventStreamKey]int),
		generation:          generation,
		config:              config,
		now:                 now,
	}
}

func applyEventHubDefaults(config *EventHubConfig) {
	if config.MaxEvents <= 0 {
		config.MaxEvents = DefaultEventCapacity
	}
	if config.ReplayWindow <= 0 {
		config.ReplayWindow = DefaultEventReplayWindow
	}
	if config.SubscriberQueueSize <= 0 {
		config.SubscriberQueueSize = DefaultEventSubscriberQueueSize
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = DefaultEventHeartbeatInterval
	}
	if config.StreamLifetime <= 0 {
		config.StreamLifetime = DefaultEventStreamLifetime
	}
	if config.MaxStreamsPerUser <= 0 {
		config.MaxStreamsPerUser = DefaultEventMaxStreamsPerUser
	}
	if config.MaxStreamsPerUserInstance <= 0 {
		config.MaxStreamsPerUserInstance = DefaultEventMaxStreamsPerInstance
	}
	if config.MaxPayloadBytes <= 0 {
		config.MaxPayloadBytes = DefaultEventMaxPayloadBytes
	}
}

func newEventGeneration() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func validGeneration(generation string) bool {
	return generation != "" && !strings.ContainsAny(generation, "\r\n\t ")
}

// Generation returns the process generation used by this hub.
func (h *EventHub) Generation() string {
	if h == nil {
		return ""
	}
	return h.generation
}

// Publish assigns a per-instance monotonic sequence and makes the event
// available to future reconnects. Subscriber delivery is always non-blocking;
// a full subscriber queue is disconnected instead.
func (h *EventHub) Publish(instanceID string, input EventInput) (Event, error) {
	if h == nil {
		return Event{}, ErrEventHubClosed
	}
	instanceID = strings.TrimSpace(instanceID)
	if err := validatePublishedEvent(instanceID, input); err != nil {
		return Event{}, err
	}
	data, err := marshalEventData(input.Data, h.config.MaxPayloadBytes)
	if err != nil {
		return Event{}, err
	}
	scope := input.Scope
	if scope.InstanceID == "" {
		scope.InstanceID = instanceID
	}
	if scope.InstanceID != instanceID {
		return Event{}, fmt.Errorf("%w: event instance does not match publish instance", ErrInvalidEvent)
	}

	now := h.now().UTC()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return Event{}, ErrEventHubClosed
	}
	instance := h.instanceLocked(instanceID)
	instance.purgeExpired(now, h.config.ReplayWindow)
	if instance.lastSequence == ^uint64(0) {
		h.mu.Unlock()
		return Event{}, fmt.Errorf("%w: event sequence exhausted", ErrInvalidEvent)
	}
	instance.lastSequence++
	event := Event{
		ID:         formatEventID(h.generation, instance.lastSequence),
		Generation: h.generation,
		Sequence:   instance.lastSequence,
		Type:       strings.TrimSpace(input.Type),
		Scope:      scope,
		Data:       cloneRawMessage(data),
		published:  now,
	}
	instance.events = append(instance.events, event)
	if len(instance.events) > h.config.MaxEvents {
		instance.events = instance.events[len(instance.events)-h.config.MaxEvents:]
	}
	subscribers := make([]*EventSubscription, 0, len(instance.subscribers))
	for subscriber := range instance.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	for _, subscriber := range subscribers {
		candidate := cloneEvent(event)
		if subscriber.filter != nil && !subscriber.filter(candidate) {
			continue
		}
		h.enqueueLocked(subscriber, candidate)
	}
	h.mu.Unlock()

	event.Data = cloneRawMessage(event.Data)
	return event, nil
}

func validatePublishedEvent(instanceID string, input EventInput) error {
	if instanceID == "" || strings.ContainsAny(instanceID, "\r\n") {
		return fmt.Errorf("%w: instance id is required", ErrInvalidEvent)
	}
	typ := strings.TrimSpace(input.Type)
	if typ == "" || strings.ContainsAny(typ, "\r\n") {
		return fmt.Errorf("%w: event type is required", ErrInvalidEvent)
	}
	return nil
}

func marshalEventData(data any, maxBytes int) (json.RawMessage, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal data: %v", ErrInvalidEvent, err)
	}
	if len(encoded) > maxBytes {
		return nil, fmt.Errorf("%w: got %d bytes, limit is %d", ErrEventPayloadTooLarge, len(encoded), maxBytes)
	}
	return json.RawMessage(encoded), nil
}

func (h *EventHub) instanceLocked(instanceID string) *eventInstance {
	instance := h.instances[instanceID]
	if instance == nil {
		instance = &eventInstance{subscribers: make(map[*EventSubscription]struct{})}
		h.instances[instanceID] = instance
	}
	return instance
}

func (i *eventInstance) purgeExpired(now time.Time, window time.Duration) {
	cutoff := now.Add(-window)
	index := 0
	for index < len(i.events) && i.events[index].publishedAt().Before(cutoff) {
		index++
	}
	if index > 0 {
		i.events = i.events[index:]
	}
}

func (e Event) publishedAt() time.Time {
	return e.published
}

func (h *EventHub) enqueueLocked(subscriber *EventSubscription, event Event) {
	select {
	case subscriber.events <- event:
	default:
		h.removeSubscriptionLocked(subscriber, ErrSlowSubscriber)
	}
}

// Subscribe creates a bounded stream and queues any complete replay range
// before returning. A cancellable context owns the subscription and releases
// it without requiring a timeout or a separate cleanup call.
func (h *EventHub) Subscribe(ctx context.Context, request EventSubscriptionRequest) (*EventSubscription, error) {
	if h == nil {
		return nil, ErrEventHubClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request.InstanceID = strings.TrimSpace(request.InstanceID)
	request.UserID = strings.TrimSpace(request.UserID)
	if request.InstanceID == "" || request.UserID == "" || strings.ContainsAny(request.InstanceID+request.UserID, "\r\n") {
		return nil, ErrInvalidSubscription
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, ErrEventHubClosed
	}
	key := eventStreamKey{userID: request.UserID, instanceID: request.InstanceID}
	if h.userStreams[request.UserID] >= h.config.MaxStreamsPerUser || h.userInstanceStreams[key] >= h.config.MaxStreamsPerUserInstance {
		h.mu.Unlock()
		return nil, ErrTooManyEventStreams
	}
	instance := h.instanceLocked(request.InstanceID)
	instance.purgeExpired(h.now().UTC(), h.config.ReplayWindow)
	replay, resyncReason := h.replayLocked(instance, request)
	queueSize := h.config.SubscriberQueueSize
	initialEvents := len(replay)
	if resyncReason != "" {
		initialEvents++
	}
	if queueSize <= initialEvents {
		queueSize = initialEvents + 1
	}
	subscriber := &EventSubscription{
		hub:        h,
		instanceID: request.InstanceID,
		userID:     request.UserID,
		events:     make(chan Event, queueSize),
		done:       make(chan struct{}),
		filter:     request.Filter,
	}
	instance.subscribers[subscriber] = struct{}{}
	h.userStreams[request.UserID]++
	h.userInstanceStreams[key]++
	if resyncReason != "" {
		h.enqueueLocked(subscriber, newResyncEvent(request.InstanceID, h.generation, resyncReason))
	} else {
		for _, event := range replay {
			h.enqueueLocked(subscriber, event)
		}
	}
	h.mu.Unlock()

	watchSubscriptionContext(ctx, subscriber)
	return subscriber, nil
}

func (h *EventHub) replayLocked(instance *eventInstance, request EventSubscriptionRequest) ([]Event, string) {
	lastID := strings.TrimSpace(request.LastEventID)
	if lastID == "" {
		return nil, ""
	}
	generation, sequence, ok := ParseEventID(lastID)
	if !ok {
		return nil, ResyncReasonInvalidCursor
	}
	if generation != h.generation {
		return nil, ResyncReasonGenerationMismatch
	}
	if sequence > instance.lastSequence {
		return nil, ResyncReasonCursorAhead
	}
	if len(instance.events) == 0 {
		if sequence < instance.lastSequence {
			return nil, ResyncReasonHistoryGap
		}
		return nil, ""
	}
	oldest := instance.events[0].Sequence
	if sequence < oldest-1 {
		return nil, ResyncReasonHistoryGap
	}
	replay := make([]Event, 0, len(instance.events))
	for _, event := range instance.events {
		if event.Sequence <= sequence {
			continue
		}
		candidate := cloneEvent(event)
		if request.Filter == nil || request.Filter(candidate) {
			replay = append(replay, candidate)
		}
	}
	return replay, ""
}

func newResyncEvent(instanceID, generation, reason string) Event {
	data, _ := json.Marshal(ResyncInfo{Reason: reason, Generation: generation, Reset: true})
	return Event{
		Generation: generation,
		Type:       RuntimeResyncRequired,
		Scope:      EventScope{InstanceID: instanceID},
		Data:       data,
	}
}

func watchSubscriptionContext(ctx context.Context, subscriber *EventSubscription) {
	if ctx.Done() == nil {
		return
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = subscriber.closeWithError(ctx.Err())
		case <-subscriber.done:
		}
	}()
}

// Events returns the replay and live event queue. It is closed when the
// subscription is canceled, explicitly closed, the hub closes, or a slow
// consumer is disconnected.
func (s *EventSubscription) Events() <-chan Event {
	if s == nil {
		return nil
	}
	return s.events
}

// Done closes with the event queue and is useful to join cancellation cleanup
// in callers that consume Events directly.
func (s *EventSubscription) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

// Err reports why the subscription was closed, if it was closed abnormally.
func (s *EventSubscription) Err() error {
	if s == nil {
		return nil
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.err
}

// IsClosed reports whether the subscription no longer accepts events.
func (s *EventSubscription) IsClosed() bool {
	if s == nil {
		return true
	}
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.closed
}

// Close releases the subscription. It is safe to call more than once.
func (s *EventSubscription) Close() error {
	return s.closeWithError(nil)
}

// Unsubscribe is an alias for Close for callers that use subscription-style
// terminology.
func (s *EventSubscription) Unsubscribe() error {
	return s.Close()
}

func (s *EventSubscription) closeWithError(err error) error {
	if s == nil || s.hub == nil {
		return nil
	}
	s.hub.mu.Lock()
	s.hub.removeSubscriptionLocked(s, err)
	s.hub.mu.Unlock()
	return nil
}

func (h *EventHub) removeSubscriptionLocked(subscriber *EventSubscription, err error) {
	instance := h.instances[subscriber.instanceID]
	if instance != nil {
		if _, active := instance.subscribers[subscriber]; active {
			delete(instance.subscribers, subscriber)
			key := eventStreamKey{userID: subscriber.userID, instanceID: subscriber.instanceID}
			decrementCount(h.userStreams, subscriber.userID)
			decrementCount(h.userInstanceStreams, key)
		}
	}
	subscriber.closeLocked(err)
}

func decrementCount[K comparable](counts map[K]int, key K) {
	if counts[key] <= 1 {
		delete(counts, key)
		return
	}
	counts[key]--
}

func (s *EventSubscription) closeLocked(err error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.err = err
	close(s.events)
	close(s.done)
}

// Close terminates the hub and all of its subscriptions. It does not wait on
// consumers or publishers because no hub operation performs a blocking send.
func (h *EventHub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for _, instance := range h.instances {
		for subscriber := range instance.subscribers {
			subscriber.closeLocked(ErrEventHubClosed)
		}
	}
	h.instances = make(map[string]*eventInstance)
	h.userStreams = make(map[string]int)
	h.userInstanceStreams = make(map[eventStreamKey]int)
}

// Handler binds an authorized instance/user/filter tuple to a standard
// net/http handler. The request's Last-Event-ID header is preferred over the
// bound request value, and no query-token handling is provided here.
func (h *EventHub) Handler(request EventSubscriptionRequest) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Serve(w, r, request)
	})
}

// Serve writes one SSE stream for an authorized subscription request.
func (h *EventHub) Serve(w http.ResponseWriter, r *http.Request, request EventSubscriptionRequest) {
	if h == nil {
		writeEventStreamError(w, http.StatusServiceUnavailable, ErrEventHubClosed)
		return
	}
	if !validEventStreamRequest(w, r) {
		return
	}
	flusher, ok := eventStreamFlusher(w)
	if !ok {
		return
	}
	request = eventStreamRequest(r, request)
	subscriber, err := h.Subscribe(r.Context(), request)
	if err != nil {
		writeEventStreamError(w, eventStreamErrorStatus(err), err)
		return
	}
	defer func() { _ = subscriber.Close() }()
	h.stream(w, r, flusher, subscriber)
}

func validEventStreamRequest(w http.ResponseWriter, r *http.Request) bool {
	if r != nil && r.Method == http.MethodGet {
		return true
	}
	if w != nil {
		w.Header().Set("Allow", http.MethodGet)
	}
	writeEventStreamError(w, http.StatusMethodNotAllowed, ErrInvalidSubscription)
	return false
}

func eventStreamFlusher(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeEventStreamError(w, http.StatusInternalServerError, errors.New("webapp: SSE requires response flushing"))
		return nil, false
	}
	return flusher, true
}

func eventStreamRequest(r *http.Request, request EventSubscriptionRequest) EventSubscriptionRequest {
	if headerID := strings.TrimSpace(r.Header.Get("Last-Event-ID")); headerID != "" {
		request.LastEventID = headerID
	}
	return request
}

func (h *EventHub) stream(w http.ResponseWriter, r *http.Request, flusher http.Flusher, subscriber *EventSubscription) {

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(h.config.HeartbeatInterval)
	defer ticker.Stop()
	var lifetime <-chan time.Time
	var timer *time.Timer
	if h.config.StreamLifetime > 0 {
		timer = time.NewTimer(h.config.StreamLifetime)
		defer timer.Stop()
		lifetime = timer.C
	}
	for {
		select {
		case event, open := <-subscriber.Events():
			if !open {
				return
			}
			if err := writeSSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-lifetime:
			return
		}
	}
}

func writeSSE(w io.Writer, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	var stream strings.Builder
	if event.ID == "" && event.Type == RuntimeResyncRequired {
		stream.WriteString("id:\n")
	} else if event.ID != "" {
		stream.WriteString("id: ")
		stream.WriteString(event.ID)
		stream.WriteByte('\n')
	}
	stream.WriteString("event: ")
	stream.WriteString(event.Type)
	stream.WriteString("\ndata: ")
	stream.Write(data)
	stream.WriteString("\n\n")
	_, err = io.WriteString(w, stream.String())
	return err
}

func eventStreamErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrEventHubClosed):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrTooManyEventStreams):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrInvalidSubscription):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeEventStreamError(w http.ResponseWriter, status int, err error) {
	if w == nil {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.Error(w, err.Error(), status)
}

// ParseEventID decodes the generation and sequence assigned by Publish.
func ParseEventID(id string) (generation string, sequence uint64, ok bool) {
	id = strings.TrimSpace(id)
	separator := strings.LastIndexByte(id, ':')
	if separator <= 0 || separator == len(id)-1 {
		return "", 0, false
	}
	generation = id[:separator]
	sequence, err := strconv.ParseUint(id[separator+1:], 10, 64)
	if err != nil || sequence == 0 || !validGeneration(generation) {
		return "", 0, false
	}
	return generation, sequence, true
}

func formatEventID(generation string, sequence uint64) string {
	return generation + ":" + strconv.FormatUint(sequence, 10)
}

func cloneEvent(event Event) Event {
	event.Data = cloneRawMessage(event.Data)
	return event
}

func cloneRawMessage(data json.RawMessage) json.RawMessage {
	if data == nil {
		return nil
	}
	return append(json.RawMessage(nil), data...)
}
