package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// Manager manages session lifecycles
type Manager struct {
	mu              sync.RWMutex
	sessions        map[types.ID]*SessionWithStateMachine
	cfg             config.SessionConfig
	logger          *logger.Logger
	store           *Store
	closed          bool
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
	wg              sync.WaitGroup
}

// New creates a new session manager
func New(cfg config.SessionConfig, log *logger.Logger) (*Manager, error) {
	if log == nil {
		var err error
		log, err = logger.NewDefault()
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to create default logger", err)
		}
	}

	// Initialize persistence store if enabled
	var store *Store
	if cfg.PersistenceEnabled {
		var err error
		store, err = NewStore(cfg, log)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInternal, "failed to initialize persistence store", err)
		}
	}

	m := &Manager{
		sessions:        make(map[types.ID]*SessionWithStateMachine),
		cfg:             cfg,
		logger:          log.With("component", "session_manager"),
		store:           store,
		closed:          false,
		cleanupInterval: time.Minute,
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine
	m.wg.Add(1)
	go m.cleanupLoop()

	m.logger.Info("Session manager initialized",
		"max_sessions", cfg.MaxSessions,
		"idle_timeout", cfg.IdleTimeout.String(),
		"timeout", cfg.Timeout.String(),
		"persistence_enabled", cfg.PersistenceEnabled)

	return m, nil
}

// NewDefault creates a new session manager with default configuration
func NewDefault(log *logger.Logger) (*Manager, error) {
	cfg := config.DefaultSessionConfig()
	return New(cfg, log)
}

// Create creates a new session with the given metadata and configuration
func (m *Manager) Create(ctx context.Context, metadata types.SessionMetadata, sessionCfg types.SessionConfig) (types.ID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return "", types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	// Check max sessions limit
	if m.cfg.MaxSessions > 0 && len(m.sessions) >= m.cfg.MaxSessions {
		return "", types.NewError(types.ErrCodeResourceExhausted, fmt.Sprintf("maximum sessions limit reached: %d", m.cfg.MaxSessions))
	}

	// Generate session ID
	sessionID := types.GenerateID()

	now := types.NewTimestampFromTime(time.Now())

	// Calculate expiration
	var expiresAt *types.Timestamp
	if sessionCfg.MaxDuration > 0 {
		expires := time.Now().Add(sessionCfg.MaxDuration)
		expiresAt = &types.Timestamp{Time: expires}
	}

	// Create session
	session := &types.Session{
		ID:        sessionID,
		State:     types.SessionStateInitializing,
		Status:    types.StatusStarting,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: expiresAt,
		Metadata:  metadata,
		Context: types.SessionContext{
			Config: sessionCfg,
		},
		Containers: []types.ID{},
	}

	// Wrap with state machine
	sessionWithSM := NewSessionWithStateMachine(session)

	// Store session
	m.sessions[sessionID] = sessionWithSM

	// Transition to active state
	if err := sessionWithSM.Activate(); err != nil {
		delete(m.sessions, sessionID)
		return "", types.WrapError(types.ErrCodeInternal, "failed to activate session", err)
	}

	session.Status = types.StatusRunning

	m.logger.Info("Session created",
		"session_id", sessionID,
		"name", metadata.Name,
		"owner_id", metadata.OwnerID,
		"expires_at", expiresAt)

	return sessionID, nil
}

// Get retrieves a session by ID
func (m *Manager) Get(ctx context.Context, sessionID types.ID) (*types.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	// Return a copy to prevent external modification
	session := sessionWithSM.Session()
	return m.copySession(session), nil
}

// List retrieves all sessions matching the filter
func (m *Manager) List(ctx context.Context, filter *types.SessionFilter) ([]*types.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	var result []*types.Session

	for _, sessionWithSM := range m.sessions {
		session := sessionWithSM.Session()

		// Apply filter
		if filter != nil && !m.matchesFilter(session, filter) {
			continue
		}

		result = append(result, m.copySession(session))
	}

	return result, nil
}

// Update updates a session's metadata
func (m *Manager) Update(ctx context.Context, sessionID types.ID, metadata types.SessionMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	// Don't allow updates to closed sessions
	if sessionWithSM.IsTerminal() {
		return types.NewError(types.ErrCodeFailedPrecondition, "cannot update closed session")
	}

	session := sessionWithSM.Session()
	session.Metadata = metadata
	session.UpdatedAt = types.NewTimestampFromTime(time.Now())

	m.logger.Debug("Session updated", "session_id", sessionID, "name", metadata.Name)
	return nil
}

// CloseSession transitions a session to closing state
func (m *Manager) CloseSession(ctx context.Context, sessionID types.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	// Already in closing/closed state
	if sessionWithSM.CurrentState() == types.SessionStateClosing ||
		sessionWithSM.CurrentState() == types.SessionStateClosed {
		return nil
	}

	// Transition to closing
	if err := sessionWithSM.Close(); err != nil {
		return types.WrapError(types.ErrCodeInternal, "failed to close session", err)
	}

	session := sessionWithSM.Session()
	session.Status = types.StatusStopping

	m.logger.Info("Session closing", "session_id", sessionID)
	return nil
}

// Delete removes a session from the manager
func (m *Manager) Delete(ctx context.Context, sessionID types.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	// Only allow deletion of closed sessions
	if !sessionWithSM.IsTerminal() {
		return types.NewError(types.ErrCodeFailedPrecondition, "cannot delete active session, close it first")
	}

	delete(m.sessions, sessionID)

	m.logger.Info("Session deleted", "session_id", sessionID)
	return nil
}

// AddContainer adds a container to a session
func (m *Manager) AddContainer(ctx context.Context, sessionID, containerID types.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	// Don't allow adding containers to closed sessions
	if sessionWithSM.IsTerminal() {
		return types.NewError(types.ErrCodeFailedPrecondition, "cannot add container to closed session")
	}

	session := sessionWithSM.Session()

	// Check max containers limit
	if session.Context.Config.MaxContainers > 0 &&
		len(session.Containers) >= session.Context.Config.MaxContainers {
		return types.NewError(types.ErrCodeResourceExhausted,
			fmt.Sprintf("maximum containers limit reached: %d", session.Context.Config.MaxContainers))
	}

	// Check for duplicates
	for _, cid := range session.Containers {
		if cid == containerID {
			return types.NewError(types.ErrCodeAlreadyExists, fmt.Sprintf("container already in session: %s", containerID))
		}
	}

	session.Containers = append(session.Containers, containerID)
	session.UpdatedAt = types.NewTimestampFromTime(time.Now())

	m.logger.Debug("Container added to session",
		"session_id", sessionID,
		"container_id", containerID)

	return nil
}

// RemoveContainer removes a container from a session
func (m *Manager) RemoveContainer(ctx context.Context, sessionID, containerID types.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	session := sessionWithSM.Session()

	// Find and remove container
	found := false
	newContainers := make([]types.ID, 0, len(session.Containers))
	for _, cid := range session.Containers {
		if cid == containerID {
			found = true
			continue
		}
		newContainers = append(newContainers, cid)
	}

	if !found {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("container not found in session: %s", containerID))
	}

	session.Containers = newContainers
	session.UpdatedAt = types.NewTimestampFromTime(time.Now())

	m.logger.Debug("Container removed from session",
		"session_id", sessionID,
		"container_id", containerID)

	return nil
}

// AddMessage adds a message to a session's conversation history
func (m *Manager) AddMessage(ctx context.Context, sessionID types.ID, message types.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	// Don't allow adding messages to closed sessions
	if sessionWithSM.IsTerminal() {
		return types.NewError(types.ErrCodeFailedPrecondition, "cannot add message to closed session")
	}

	session := sessionWithSM.Session()

	// Ensure message has ID and timestamp
	if message.ID.IsEmpty() {
		message.ID = types.GenerateID()
	}
	if message.Timestamp.IsZero() {
		message.Timestamp = types.NewTimestampFromTime(time.Now())
	}

	session.Context.Messages = append(session.Context.Messages, message)
	session.UpdatedAt = types.NewTimestampFromTime(time.Now())

	// Transition from idle to active when a message is added
	if sessionWithSM.CurrentState() == types.SessionStateIdle {
		_ = sessionWithSM.Activate()
	}

	m.logger.Debug("Message added to session",
		"session_id", sessionID,
		"message_id", message.ID,
		"role", message.Role)

	return nil
}

// GetState returns the current state of a session
func (m *Manager) GetState(ctx context.Context, sessionID types.ID) (types.SessionState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return "", types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return "", types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	return sessionWithSM.CurrentState(), nil
}

// GetStats returns statistics for a session
func (m *Manager) GetStats(ctx context.Context, sessionID types.ID) (*types.SessionStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return nil, types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	session := sessionWithSM.Session()

	stats := &types.SessionStats{
		SessionID:       sessionID,
		MessageCount:    len(session.Context.Messages),
		ContainerCount:  len(session.Containers),
		TaskCount:       len(session.Context.TaskHistory),
		DurationSeconds: int64(time.Since(session.CreatedAt.Time).Seconds()),
		Timestamp:       types.NewTimestampFromTime(time.Now()),
	}

	return stats, nil
}

// Close closes the session manager and cleans up all sessions
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	// Stop cleanup goroutine
	close(m.stopCleanup)
	m.wg.Wait()

	// Close all active sessions
	for sessionID, sessionWithSM := range m.sessions {
		if !sessionWithSM.IsTerminal() {
			session := sessionWithSM.Session()
			_ = sessionWithSM.ForceClose()
			session.Status = types.StatusStopped
			m.logger.Info("Session force closed during manager shutdown", "session_id", sessionID)
		}
	}

	// Close persistence store if initialized
	if m.store != nil {
		if err := m.store.Close(); err != nil {
			m.logger.Warn("Failed to close persistence store", "error", err)
		}
	}

	m.closed = true
	m.logger.Info("Session manager closed")
	return nil
}

// Stats returns manager statistics
func (m *Manager) Stats(ctx context.Context) (active, idle, closing, closed int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, sessionWithSM := range m.sessions {
		switch sessionWithSM.CurrentState() {
		case types.SessionStateActive:
			active++
		case types.SessionStateIdle:
			idle++
		case types.SessionStateClosing:
			closing++
		case types.SessionStateClosed:
			closed++
		}
	}

	return
}

// cleanupLoop runs periodic cleanup of expired and idle sessions
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCleanup:
			return
		}
	}
}

// cleanup removes expired sessions and transitions idle sessions
func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	for sessionID, sessionWithSM := range m.sessions {
		session := sessionWithSM.Session()

		// Skip terminal sessions
		if sessionWithSM.IsTerminal() {
			continue
		}

		// Check expiration
		if session.ExpiresAt != nil && now.After(session.ExpiresAt.Time) {
			m.logger.Info("Session expired, closing", "session_id", sessionID)
			_ = sessionWithSM.Close()
			session.Status = types.StatusStopping
			continue
		}

		// Check idle timeout
		if m.cfg.IdleTimeout > 0 && sessionWithSM.CurrentState() == types.SessionStateActive {
			idleTime := now.Sub(session.UpdatedAt.Time)
			if idleTime > m.cfg.IdleTimeout {
				m.logger.Info("Session idle timeout, transitioning to idle", "session_id", sessionID, "idle_time", idleTime)
				_ = sessionWithSM.Idle()
				session.Status = types.StatusRunning
			}
		}
	}
}

// matchesFilter checks if a session matches the given filter
func (m *Manager) matchesFilter(session *types.Session, filter *types.SessionFilter) bool {
	if filter.State != nil && session.State != *filter.State {
		return false
	}

	if filter.Status != nil && session.Status != *filter.Status {
		return false
	}

	if filter.OwnerID != nil && session.Metadata.OwnerID != *filter.OwnerID {
		return false
	}

	if filter.Label != nil {
		for key, value := range filter.Label {
			if session.Metadata.Labels == nil {
				return false
			}
			if session.Metadata.Labels[key] != value {
				return false
			}
		}
	}

	return true
}

// copySession creates a shallow copy of a session
func (m *Manager) copySession(session *types.Session) *types.Session {
	copied := *session

	// Copy slices
	if session.Containers != nil {
		copied.Containers = make([]types.ID, len(session.Containers))
		copy(copied.Containers, session.Containers)
	}

	if session.Context.Messages != nil {
		copied.Context.Messages = make([]types.Message, len(session.Context.Messages))
		copy(copied.Context.Messages, session.Context.Messages)
	}

	if session.Context.TaskHistory != nil {
		copied.Context.TaskHistory = make([]types.ID, len(session.Context.TaskHistory))
		copy(copied.Context.TaskHistory, session.Context.TaskHistory)
	}

	if session.Metadata.Tags != nil {
		copied.Metadata.Tags = make([]string, len(session.Metadata.Tags))
		copy(copied.Metadata.Tags, session.Metadata.Tags)
	}

	if session.Metadata.Labels != nil {
		copied.Metadata.Labels = make(map[string]string)
		for k, v := range session.Metadata.Labels {
			copied.Metadata.Labels[k] = v
		}
	}

	return &copied
}

// GetActiveContainerIDs returns all container IDs across all active sessions
func (m *Manager) GetActiveContainerIDs(ctx context.Context) ([]types.ID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	var containerIDs []types.ID
	for _, sessionWithSM := range m.sessions {
		if sessionWithSM.IsActive() {
			session := sessionWithSM.Session()
			containerIDs = append(containerIDs, session.Containers...)
		}
	}

	return containerIDs, nil
}

// ValidateSession checks if a session exists and is active
func (m *Manager) ValidateSession(ctx context.Context, sessionID types.ID) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return types.NewError(types.ErrCodeUnavailable, "session manager is closed")
	}

	sessionWithSM, exists := m.sessions[sessionID]
	if !exists {
		return types.NewError(types.ErrCodeNotFound, fmt.Sprintf("session not found: %s", sessionID))
	}

	if !sessionWithSM.IsActive() {
		return types.NewError(types.ErrCodeFailedPrecondition, fmt.Sprintf("session is not active: %s (state: %s)", sessionID, sessionWithSM.CurrentState()))
	}

	// Check expiration
	session := sessionWithSM.Session()
	if session.ExpiresAt != nil && time.Now().After(session.ExpiresAt.Time) {
		return types.NewError(types.ErrCodeFailedPrecondition, fmt.Sprintf("session has expired: %s", sessionID))
	}

	return nil
}

// String returns a string representation of the manager
func (m *Manager) String() string {
	active, idle, closing, closed := m.Stats(context.Background())
	return fmt.Sprintf("SessionManager{active: %d, idle: %d, closing: %d, closed: %d, total: %d}",
		active, idle, closing, closed, len(m.sessions))
}

// GetManagerConfig returns the manager's configuration
func (m *Manager) GetManagerConfig() config.SessionConfig {
	return m.cfg
}
