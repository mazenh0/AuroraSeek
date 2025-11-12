package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed means the circuit is closed (normal operation)
	StateClosed State = iota
	// StateOpen means the circuit is open (failing, reject requests)
	StateOpen
	// StateHalfOpen means the circuit is half-open (testing if service recovered)
	StateHalfOpen
)

// ErrCircuitOpen is returned when the circuit is open
var ErrCircuitOpen = errors.New("circuit breaker is open")

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config configures the circuit breaker
type Config struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes before closing the circuit
	SuccessThreshold int
	// Timeout is the duration to wait before transitioning from open to half-open
	Timeout time.Duration
	// HalfOpenMaxRequests is the maximum number of requests to allow in half-open state
	HalfOpenMaxRequests int
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config          Config
	state           State
	failures        int
	successes       int
	lastFailureTime time.Time
	halfOpenRequests int
	mu              sync.RWMutex
	onStateChange   func(from, to State)
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config Config) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// SetOnStateChange sets a callback for state changes
func (cb *CircuitBreaker) SetOnStateChange(fn func(from, to State)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	state := cb.state

	// Check if circuit is open
	if state == StateOpen {
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			// Transition to half-open
			cb.setState(StateHalfOpen)
			cb.halfOpenRequests = 0
			state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}

	// Check if circuit is half-open and we've exceeded max requests
	if state == StateHalfOpen {
		if cb.halfOpenRequests >= cb.config.HalfOpenMaxRequests {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
		cb.halfOpenRequests++
	}

	cb.mu.Unlock()

	// Execute the function
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}

	return err
}

// onFailure handles a failure
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.successes = 0
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		// Any failure in half-open state opens the circuit
		cb.setState(StateOpen)
		cb.halfOpenRequests = 0
	}
}

// onSuccess handles a success
func (cb *CircuitBreaker) onSuccess() {
	cb.failures = 0
	cb.successes++

	switch cb.state {
	case StateHalfOpen:
		if cb.successes >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
			cb.halfOpenRequests = 0
		}
	case StateClosed:
		// Reset failures on success in closed state
		cb.failures = 0
	}
}

// setState sets the circuit breaker state
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state != newState {
		oldState := cb.state
		cb.state = newState
		if cb.onStateChange != nil {
			cb.onStateChange(oldState, newState)
		}
	}
}

// State returns the current state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.setState(StateClosed)
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() (failures, successes int, state State) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures, cb.successes, cb.state
}

