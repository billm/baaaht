package session

import (
	"context"
	"testing"
	"time"

	"github.com/billm/baaaht/orchestrator/internal/config"
	"github.com/billm/baaaht/orchestrator/internal/logger"
	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// TestNewManager tests creating a new session manager
func TestNewManager(t *testing.T) {
	log, err := logger.NewDefault()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	cfg := config.DefaultSessionConfig()

	manager, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	if manager == nil {
		t.Fatal("manager is nil")
	}
}

// TestNewManagerNilLogger tests creating a manager with nil logger
func TestNewManagerNilLogger(t *testing.T) {
	cfg := config.DefaultSessionConfig()

	manager, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create manager with nil logger: %v", err)
	}
	defer manager.Close()

	if manager == nil {
		t.Fatal("manager is nil")
	}
}

// TestNewDefaultManager tests creating a manager with default config
func TestNewDefaultManager(t *testing.T) {
	manager, err := NewDefault(nil)
	if err != nil {
		t.Fatalf("failed to create default manager: %v", err)
	}
	defer manager.Close()

	if manager == nil {
		t.Fatal("manager is nil")
	}
}

// TestManagerCreate tests creating a new session
func TestManagerCreate(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()
	metadata := types.SessionMetadata{
		Name:    "test-session",
		OwnerID: "user-123",
		Labels: map[string]string{
			"env": "test",
		},
	}

	sessionCfg := types.SessionConfig{
		MaxContainers: 5,
		IdleTimeout:   5 * time.Minute,
		MaxDuration:   time.Hour,
	}

	sessionID, err := manager.Create(ctx, metadata, sessionCfg)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if sessionID.IsEmpty() {
		t.Fatal("session ID is empty")
	}

	// Verify session was created
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if session.ID != sessionID {
		t.Errorf("session ID mismatch: got %s, want %s", session.ID, sessionID)
	}

	if session.State != types.SessionStateActive {
		t.Errorf("session state: got %s, want %s", session.State, types.SessionStateActive)
	}

	if session.Metadata.Name != "test-session" {
		t.Errorf("session name: got %s, want test-session", session.Metadata.Name)
	}

	if session.Status != types.StatusRunning {
		t.Errorf("session status: got %s, want %s", session.Status, types.StatusRunning)
	}
}

// TestManagerCreateMaxSessions tests the max sessions limit
func TestManagerCreateMaxSessions(t *testing.T) {
	cfg := config.DefaultSessionConfig()
	cfg.MaxSessions = 2

	log, _ := logger.NewDefault()
	manager, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	ctx := context.Background()
	metadata := types.SessionMetadata{Name: "test"}
	sessionCfg := types.SessionConfig{}

	// Create first session
	_, err = manager.Create(ctx, metadata, sessionCfg)
	if err != nil {
		t.Fatalf("failed to create first session: %v", err)
	}

	// Create second session
	_, err = manager.Create(ctx, metadata, sessionCfg)
	if err != nil {
		t.Fatalf("failed to create second session: %v", err)
	}

	// Try to create third session (should fail)
	_, err = manager.Create(ctx, metadata, sessionCfg)
	if err == nil {
		t.Fatal("expected error when creating session beyond limit, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeResourceExhausted) {
		t.Errorf("expected resource exhausted error, got: %v", err)
	}
}

// TestManagerGet tests getting a session
func TestManagerGet(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	// Get non-existent session
	_, err := manager.Get(ctx, "non-existent")
	if err == nil {
		t.Fatal("expected error when getting non-existent session, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeNotFound) {
		t.Errorf("expected not found error, got: %v", err)
	}

	// Create a session and get it
	sessionID := createTestSession(t, manager)
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if session.ID != sessionID {
		t.Errorf("session ID mismatch: got %s, want %s", session.ID, sessionID)
	}
}

// TestManagerList tests listing sessions
func TestManagerList(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	// List with no sessions
	sessions, err := manager.List(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}

	// Create sessions
	sessionID1 := createTestSessionWithOwner(t, manager, "user-1")
	sessionID2 := createTestSessionWithOwner(t, manager, "user-2")
	_ = createTestSessionWithOwner(t, manager, "user-1")

	// List all sessions
	sessions, err = manager.List(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}

	// Filter by owner
	filter := &types.SessionFilter{}
	ownerID := "user-1"
	filter.OwnerID = &ownerID

	sessions, err = manager.List(ctx, filter)
	if err != nil {
		t.Fatalf("failed to list sessions with filter: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions for user-1, got %d", len(sessions))
	}

	// Filter by state
	state := types.SessionStateActive
	filter.State = &state
	filter.OwnerID = nil

	sessions, err = manager.List(ctx, filter)
	if err != nil {
		t.Fatalf("failed to list sessions by state: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("expected 3 active sessions, got %d", len(sessions))
	}
}

// TestManagerUpdate tests updating a session
func TestManagerUpdate(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)

	// Update session metadata
	newMetadata := types.SessionMetadata{
		Name:    "updated-session",
		OwnerID: "user-123",
		Labels: map[string]string{
			"env":     "production",
			"updated": "true",
		},
	}

	err := manager.Update(ctx, sessionID, newMetadata)
	if err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	// Verify update
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if session.Metadata.Name != "updated-session" {
		t.Errorf("session name: got %s, want updated-session", session.Metadata.Name)
	}

	if session.Metadata.Labels["env"] != "production" {
		t.Errorf("session label env: got %s, want production", session.Metadata.Labels["env"])
	}
}

// TestManagerClose tests closing a session
func TestManagerClose(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)

	// Close session
	err := manager.Close(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to close session: %v", err)
	}

	// Verify state
	state, err := manager.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session state: %v", err)
	}

	if state != types.SessionStateClosing {
		t.Errorf("session state: got %s, want %s", state, types.SessionStateClosing)
	}

	// Close again should be idempotent
	err = manager.Close(ctx, sessionID)
	if err != nil {
		t.Errorf("close should be idempotent, got error: %v", err)
	}
}

// TestManagerDelete tests deleting a session
func TestManagerDelete(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)

	// Try to delete active session (should fail)
	err := manager.Delete(ctx, sessionID)
	if err == nil {
		t.Fatal("expected error when deleting active session, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeFailedPrecondition) {
		t.Errorf("expected failed precondition error, got: %v", err)
	}

	// Close the session
	_ = manager.Close(ctx, sessionID)

	// Transition to closed state
	session, _ := manager.Get(ctx, sessionID)
	session.State = types.SessionStateClosed

	// Delete the session
	err = manager.Delete(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Verify session is gone
	_, err = manager.Get(ctx, sessionID)
	if err == nil {
		t.Fatal("expected error when getting deleted session, got nil")
	}
}

// TestManagerAddContainer tests adding a container to a session
func TestManagerAddContainer(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)
	containerID := types.GenerateID()

	// Add container
	err := manager.AddContainer(ctx, sessionID, containerID)
	if err != nil {
		t.Fatalf("failed to add container: %v", err)
	}

	// Verify container was added
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if len(session.Containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(session.Containers))
	}

	if session.Containers[0] != containerID {
		t.Errorf("container ID: got %s, want %s", session.Containers[0], containerID)
	}

	// Add duplicate container (should fail)
	err = manager.AddContainer(ctx, sessionID, containerID)
	if err == nil {
		t.Fatal("expected error when adding duplicate container, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeAlreadyExists) {
		t.Errorf("expected already exists error, got: %v", err)
	}
}

// TestManagerMaxContainers tests the max containers limit
func TestManagerMaxContainers(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	metadata := types.SessionMetadata{Name: "test"}
	sessionCfg := types.SessionConfig{
		MaxContainers: 2,
	}

	sessionID, _ := manager.Create(ctx, metadata, sessionCfg)

	// Add containers up to limit
	_ = manager.AddContainer(ctx, sessionID, types.GenerateID())
	_ = manager.AddContainer(ctx, sessionID, types.GenerateID())

	// Try to add one more (should fail)
	err := manager.AddContainer(ctx, sessionID, types.GenerateID())
	if err == nil {
		t.Fatal("expected error when exceeding max containers, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeResourceExhausted) {
		t.Errorf("expected resource exhausted error, got: %v", err)
	}
}

// TestManagerRemoveContainer tests removing a container from a session
func TestManagerRemoveContainer(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)
	containerID := types.GenerateID()

	_ = manager.AddContainer(ctx, sessionID, containerID)

	// Remove container
	err := manager.RemoveContainer(ctx, sessionID, containerID)
	if err != nil {
		t.Fatalf("failed to remove container: %v", err)
	}

	// Verify container was removed
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if len(session.Containers) != 0 {
		t.Errorf("expected 0 containers, got %d", len(session.Containers))
	}

	// Remove non-existent container
	err = manager.RemoveContainer(ctx, sessionID, containerID)
	if err == nil {
		t.Fatal("expected error when removing non-existent container, got nil")
	}
}

// TestManagerAddMessage tests adding a message to a session
func TestManagerAddMessage(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)

	message := types.Message{
		Role:    types.MessageRoleUser,
		Content: "Hello, world!",
	}

	// Add message
	err := manager.AddMessage(ctx, sessionID, message)
	if err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	// Verify message was added
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if len(session.Context.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(session.Context.Messages))
	}

	addedMessage := session.Context.Messages[0]
	if addedMessage.Role != types.MessageRoleUser {
		t.Errorf("message role: got %s, want %s", addedMessage.Role, types.MessageRoleUser)
	}

	if addedMessage.Content != "Hello, world!" {
		t.Errorf("message content: got %s, want 'Hello, world!'", addedMessage.Content)
	}

	// Verify ID and timestamp were auto-generated
	if addedMessage.ID.IsEmpty() {
		t.Error("message ID was not generated")
	}

	if addedMessage.Timestamp.IsZero() {
		t.Error("message timestamp was not generated")
	}
}

// TestManagerGetStats tests getting session statistics
func TestManagerGetStats(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)
	containerID := types.GenerateID()

	_ = manager.AddContainer(ctx, sessionID, containerID)

	message := types.Message{
		Role:    types.MessageRoleUser,
		Content: "Test message",
	}
	_ = manager.AddMessage(ctx, sessionID, message)

	// Get stats
	stats, err := manager.GetStats(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.SessionID != sessionID {
		t.Errorf("stats session ID: got %s, want %s", stats.SessionID, sessionID)
	}

	if stats.MessageCount != 1 {
		t.Errorf("stats message count: got %d, want 1", stats.MessageCount)
	}

	if stats.ContainerCount != 1 {
		t.Errorf("stats container count: got %d, want 1", stats.ContainerCount)
	}

	if stats.DurationSeconds <= 0 {
		t.Error("stats duration should be positive")
	}
}

// TestManagerStats tests manager statistics
func TestManagerStats(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	// Initial stats
	active, idle, closing, closed := manager.Stats(ctx)
	if active != 0 || idle != 0 || closing != 0 || closed != 0 {
		t.Errorf("initial stats should all be 0, got active=%d idle=%d closing=%d closed=%d",
			active, idle, closing, closed)
	}

	// Create some sessions
	sessionID1 := createTestSession(t, manager)
	sessionID2 := createTestSession(t, manager)

	// Close one session
	_ = manager.Close(ctx, sessionID2)
	session, _ := manager.Get(ctx, sessionID2)
	session.State = types.SessionStateClosed

	active, idle, closing, closed = manager.Stats(ctx)
	if active != 1 || closed != 1 {
		t.Errorf("stats after close: active=%d idle=%d closing=%d closed=%d, want active=1 closed=1",
			active, idle, closing, closed)
	}
}

// TestManagerValidateSession tests session validation
func TestManagerValidateSession(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()

	sessionID := createTestSession(t, manager)

	// Validate active session
	err := manager.ValidateSession(ctx, sessionID)
	if err != nil {
		t.Errorf("failed to validate active session: %v", err)
	}

	// Close session
	_ = manager.Close(ctx, sessionID)

	// Validate closed session (should fail)
	err = manager.ValidateSession(ctx, sessionID)
	if err == nil {
		t.Fatal("expected error when validating closed session, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeFailedPrecondition) {
		t.Errorf("expected failed precondition error, got: %v", err)
	}

	// Validate non-existent session
	err = manager.ValidateSession(ctx, "non-existent")
	if err == nil {
		t.Fatal("expected error when validating non-existent session, got nil")
	}
}

// TestManagerString tests the string representation
func TestManagerString(t *testing.T) {
	manager := createTestManager()
	defer manager.Close()

	str := manager.String()
	if str == "" {
		t.Fatal("string representation should not be empty")
	}

	// Create a session
	_ = createTestSession(t, manager)

	str = manager.String()
	t.Logf("Manager string: %s", str)
}

// TestManagerClosedOperations tests operations on a closed manager
func TestManagerClosedOperations(t *testing.T) {
	cfg := config.DefaultSessionConfig()
	log, _ := logger.NewDefault()

	manager, _ := New(cfg, log)
	_ = manager.Close()

	ctx := context.Background()

	// Try to create session on closed manager
	_, err := manager.Create(ctx, types.SessionMetadata{}, types.SessionConfig{})
	if err == nil {
		t.Fatal("expected error when creating session on closed manager, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeUnavailable) {
		t.Errorf("expected unavailable error, got: %v", err)
	}

	// Try to get session on closed manager
	_, err = manager.Get(ctx, "any-id")
	if err == nil {
		t.Fatal("expected error when getting session on closed manager, got nil")
	}
}

// TestStateMachine tests the state machine
func TestStateMachine(t *testing.T) {
	sm := NewStateMachine()

	if sm.Current() != types.SessionStateInitializing {
		t.Errorf("initial state: got %s, want %s", sm.Current(), types.SessionStateInitializing)
	}

	// Valid transition: initializing -> active
	err := sm.Transition(types.SessionStateActive)
	if err != nil {
		t.Fatalf("failed to transition to active: %v", err)
	}

	if sm.Current() != types.SessionStateActive {
		t.Errorf("current state: got %s, want %s", sm.Current(), types.SessionStateActive)
	}

	// Valid transition: active -> idle
	err = sm.Transition(types.SessionStateIdle)
	if err != nil {
		t.Fatalf("failed to transition to idle: %v", err)
	}

	// Valid transition: idle -> closing
	err = sm.Transition(types.SessionStateClosing)
	if err != nil {
		t.Fatalf("failed to transition to closing: %v", err)
	}

	// Valid transition: closing -> closed
	err = sm.Transition(types.SessionStateClosed)
	if err != nil {
		t.Fatalf("failed to transition to closed: %v", err)
	}

	if !sm.IsTerminal() {
		t.Error("closed state should be terminal")
	}
}

// TestStateMachineInvalidTransition tests invalid state transitions
func TestStateMachineInvalidTransition(t *testing.T) {
	sm := NewStateMachine()

	// Invalid transition: initializing -> idle
	err := sm.Transition(types.SessionStateIdle)
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}

	if !types.IsErrCode(err, types.ErrCodeInvalidArgument) {
		t.Errorf("expected invalid argument error, got: %v", err)
	}

	// Transition to active
	_ = sm.Transition(types.SessionStateActive)

	// Invalid transition: active -> initializing
	err = sm.Transition(types.SessionStateInitializing)
	if err == nil {
		t.Fatal("expected error for invalid transition, got nil")
	}
}

// TestStateMachineIdempotent tests that transitioning to the same state is idempotent
func TestStateMachineIdempotent(t *testing.T) {
	sm := NewStateMachine()

	_ = sm.Transition(types.SessionStateActive)

	// Transition to same state should not error
	err := sm.Transition(types.SessionStateActive)
	if err != nil {
		t.Errorf("idempotent transition should not error: %v", err)
	}

	if sm.Current() != types.SessionStateActive {
		t.Errorf("state should still be active, got %s", sm.Current())
	}
}

// TestStateMachineValidTransitions tests ValidTransitions method
func TestStateMachineValidTransitions(t *testing.T) {
	sm := NewStateMachine()

	valid := sm.ValidTransitions()
	if len(valid) != 2 {
		t.Errorf("expected 2 valid transitions from initializing, got %d", len(valid))
	}

	_ = sm.Transition(types.SessionStateActive)

	valid = sm.ValidTransitions()
	if len(valid) != 3 {
		t.Errorf("expected 3 valid transitions from active, got %d", len(valid))
	}
}

// TestSessionWithStateMachine tests SessionWithStateMachine
func TestSessionWithStateMachine(t *testing.T) {
	session := &types.Session{
		ID:      types.GenerateID(),
		State:   types.SessionStateInitializing,
		Status:  types.StatusStarting,
		CreatedAt: types.Timestamp(time.Now()),
		UpdatedAt: types.Timestamp(time.Now()),
	}

	sws := NewSessionWithStateMachine(session)

	if sws.CurrentState() != types.SessionStateInitializing {
		t.Errorf("initial state: got %s, want %s", sws.CurrentState(), types.SessionStateInitializing)
	}

	// Test Activate
	err := sws.Activate()
	if err != nil {
		t.Fatalf("failed to activate: %v", err)
	}

	if session.State != types.SessionStateActive {
		t.Errorf("session state: got %s, want %s", session.State, types.SessionStateActive)
	}

	// Test Idle
	err = sws.Idle()
	if err != nil {
		t.Fatalf("failed to idle: %v", err)
	}

	if session.State != types.SessionStateIdle {
		t.Errorf("session state: got %s, want %s", session.State, types.SessionStateIdle)
	}

	// Test Close
	err = sws.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if session.State != types.SessionStateClosing {
		t.Errorf("session state: got %s, want %s", session.State, types.SessionStateClosing)
	}

	// Test ForceClose
	err = sws.ForceClose()
	if err != nil {
		t.Fatalf("failed to force close: %v", err)
	}

	if session.State != types.SessionStateClosed {
		t.Errorf("session state: got %s, want %s", session.State, types.SessionStateClosed)
	}

	if session.ClosedAt == nil {
		t.Error("ClosedAt should be set when transitioning to closed")
	}
}

// Helper functions

func createTestManager() *Manager {
	cfg := config.DefaultSessionConfig()
	log, _ := logger.NewDefault()
	manager, _ := New(cfg, log)
	return manager
}

func createTestSession(t *testing.T, manager *Manager) types.ID {
	ctx := context.Background()
	metadata := types.SessionMetadata{
		Name:    "test-session",
		OwnerID: "user-123",
	}
	sessionCfg := types.SessionConfig{}

	sessionID, err := manager.Create(ctx, metadata, sessionCfg)
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	return sessionID
}

func createTestSessionWithOwner(t *testing.T, manager *Manager, ownerID string) types.ID {
	ctx := context.Background()
	metadata := types.SessionMetadata{
		Name:    "test-session",
		OwnerID: ownerID,
	}
	sessionCfg := types.SessionConfig{}

	sessionID, err := manager.Create(ctx, metadata, sessionCfg)
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	return sessionID
}

// BenchmarkManagerCreate benchmarks session creation
func BenchmarkManagerCreate(b *testing.B) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()
	metadata := types.SessionMetadata{Name: "bench-session"}
	sessionCfg := types.SessionConfig{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.Create(ctx, metadata, sessionCfg)
		if err != nil {
			b.Fatalf("failed to create session: %v", err)
		}
	}
}

// BenchmarkManagerGet benchmarks session retrieval
func BenchmarkManagerGet(b *testing.B) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()
	metadata := types.SessionMetadata{Name: "bench-session"}
	sessionCfg := types.SessionConfig{}

	// Create some sessions
	sessionIDs := make([]types.ID, 100)
	for i := 0; i < 100; i++ {
		sessionIDs[i], _ = manager.Create(ctx, metadata, sessionCfg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.Get(ctx, sessionIDs[i%100])
		if err != nil {
			b.Fatalf("failed to get session: %v", err)
		}
	}
}

// BenchmarkManagerAddMessage benchmarks adding messages
func BenchmarkManagerAddMessage(b *testing.B) {
	manager := createTestManager()
	defer manager.Close()

	ctx := context.Background()
	sessionID := createTestSession(b, manager)

	message := types.Message{
		Role:    types.MessageRoleUser,
		Content: "Benchmark message",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := manager.AddMessage(ctx, sessionID, message)
		if err != nil {
			b.Fatalf("failed to add message: %v", err)
		}
	}
}
