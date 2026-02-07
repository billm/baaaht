package session

import (
	"fmt"
	"time"

	"github.com/billm/baaaht/orchestrator/pkg/types"
)

// StateMachine manages session state transitions with validation
type StateMachine struct {
	current types.SessionState
}

// NewStateMachine creates a new state machine in the initializing state
func NewStateMachine() *StateMachine {
	return &StateMachine{
		current: types.SessionStateInitializing,
	}
}

// Current returns the current state
func (sm *StateMachine) Current() types.SessionState {
	return sm.current
}

// CanTransition checks if a transition to the target state is valid
func (sm *StateMachine) CanTransition(target types.SessionState) bool {
	transitions := map[types.SessionState][]types.SessionState{
		types.SessionStateInitializing: {
			types.SessionStateActive,
			types.SessionStateClosed,
		},
		types.SessionStateActive: {
			types.SessionStateIdle,
			types.SessionStateClosing,
			types.SessionStateClosed,
		},
		types.SessionStateIdle: {
			types.SessionStateActive,
			types.SessionStateClosing,
			types.SessionStateClosed,
		},
		types.SessionStateClosing: {
			types.SessionStateClosed,
		},
		types.SessionStateClosed: {}, // Terminal state
	}

	allowed, exists := transitions[sm.current]
	if !exists {
		return false
	}

	for _, allowedState := range allowed {
		if allowedState == target {
			return true
		}
	}

	return false
}

// Transition attempts to transition to the target state
func (sm *StateMachine) Transition(target types.SessionState) error {
	if sm.current == target {
		return nil // Already in target state
	}

	if !sm.CanTransition(target) {
		return types.NewError(
			types.ErrCodeInvalidArgument,
			fmt.Sprintf("invalid state transition: %s -> %s", sm.current, target),
		)
	}

	sm.current = target
	return nil
}

// MustTransition transitions to the target state or panics
// Use this for transitions that should never fail in normal operation
func (sm *StateMachine) MustTransition(target types.SessionState) {
	if err := sm.Transition(target); err != nil {
		panic(err)
	}
}

// IsTerminal returns true if the current state is terminal (closed)
func (sm *StateMachine) IsTerminal() bool {
	return sm.current == types.SessionStateClosed
}

// IsActive returns true if the session is in an active state (active or idle)
func (sm *StateMachine) IsActive() bool {
	return sm.current == types.SessionStateActive || sm.current == types.SessionStateIdle
}

// String returns the string representation of the current state
func (sm *StateMachine) String() string {
	return string(sm.current)
}

// ValidTransitions returns all valid transitions from the current state
func (sm *StateMachine) ValidTransitions() []types.SessionState {
	transitions := map[types.SessionState][]types.SessionState{
		types.SessionStateInitializing: {
			types.SessionStateActive,
			types.SessionStateClosed,
		},
		types.SessionStateActive: {
			types.SessionStateIdle,
			types.SessionStateClosing,
			types.SessionStateClosed,
		},
		types.SessionStateIdle: {
			types.SessionStateActive,
			types.SessionStateClosing,
			types.SessionStateClosed,
		},
		types.SessionStateClosing: {
			types.SessionStateClosed,
		},
		types.SessionStateClosed: {},
	}

	valid, exists := transitions[sm.current]
	if !exists {
		return []types.SessionState{}
	}

	result := make([]types.SessionState, len(valid))
	copy(result, valid)
	return result
}

// SessionWithStateMachine wraps a session with state machine operations
type SessionWithStateMachine struct {
	session *types.Session
	sm      *StateMachine
}

// NewSessionWithStateMachine creates a new session with state machine
func NewSessionWithStateMachine(session *types.Session) *SessionWithStateMachine {
	return &SessionWithStateMachine{
		session: session,
		sm:      NewStateMachine(),
	}
}

// Session returns the underlying session
func (s *SessionWithStateMachine) Session() *types.Session {
	return s.session
}

// StateMachine returns the state machine
func (s *SessionWithStateMachine) StateMachine() *StateMachine {
	return s.sm
}

// Transition attempts to transition the session to a new state
func (s *SessionWithStateMachine) Transition(target types.SessionState) error {
	if err := s.sm.Transition(target); err != nil {
		return err
	}

	s.session.State = target
		s.session.UpdatedAt = types.NewTimestampFromTime(time.Now())

	// Set closed timestamp when transitioning to closed
	if target == types.SessionStateClosed {
		now := types.NewTimestampFromTime(time.Now())
		s.session.ClosedAt = &now
	}

	return nil
}

// CanTransition checks if a transition is valid
func (s *SessionWithStateMachine) CanTransition(target types.SessionState) bool {
	return s.sm.CanTransition(target)
}

// CurrentState returns the current state
func (s *SessionWithStateMachine) CurrentState() types.SessionState {
	return s.sm.Current()
}

// Activate transitions the session to active state
func (s *SessionWithStateMachine) Activate() error {
	return s.Transition(types.SessionStateActive)
}

// Idle transitions the session to idle state
func (s *SessionWithStateMachine) Idle() error {
	return s.Transition(types.SessionStateIdle)
}

// Close transitions the session to closing state
func (s *SessionWithStateMachine) Close() error {
	return s.Transition(types.SessionStateClosing)
}

// ForceClose transitions the session directly to closed state
// Use this for immediate termination without cleanup
func (s *SessionWithStateMachine) ForceClose() error {
	return s.Transition(types.SessionStateClosed)
}

// IsTerminal returns true if the session is in a terminal state
func (s *SessionWithStateMachine) IsTerminal() bool {
	return s.sm.IsTerminal()
}

// IsActive returns true if the session is active or idle
func (s *SessionWithStateMachine) IsActive() bool {
	return s.sm.IsActive()
}
