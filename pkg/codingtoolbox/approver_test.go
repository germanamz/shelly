package codingtoolbox_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/germanamz/shelly/pkg/codingtoolbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprover_AlreadyApproved(t *testing.T) {
	a := codingtoolbox.NewApprover()

	called := false
	err := a.Ensure(context.Background(), "k",
		func() bool { return true },
		func(context.Context) codingtoolbox.ApprovalOutcome {
			called = true
			return codingtoolbox.ApprovalOutcome{Shared: true}
		},
		nil,
	)

	require.NoError(t, err)
	assert.False(t, called, "approveFn should not be called when already approved")
}

func TestApprover_ApproveFnCalled(t *testing.T) {
	a := codingtoolbox.NewApprover()

	approved := false
	err := a.Ensure(context.Background(), "k",
		func() bool { return approved },
		func(context.Context) codingtoolbox.ApprovalOutcome {
			approved = true
			return codingtoolbox.ApprovalOutcome{Shared: true}
		},
		nil,
	)

	require.NoError(t, err)
	assert.True(t, approved)
}

func TestApprover_ApproveFnError(t *testing.T) {
	a := codingtoolbox.NewApprover()

	err := a.Ensure(context.Background(), "k",
		func() bool { return false },
		func(context.Context) codingtoolbox.ApprovalOutcome {
			return codingtoolbox.ApprovalOutcome{Err: fmt.Errorf("denied")}
		},
		nil,
	)

	assert.EqualError(t, err, "denied")
}

func TestApprover_CoalesceShared(t *testing.T) {
	a := codingtoolbox.NewApprover()

	var calls atomic.Int32
	approved := atomic.Bool{}

	start := make(chan struct{})
	var wg sync.WaitGroup

	for range 5 {
		wg.Go(func() {
			<-start

			err := a.Ensure(context.Background(), "k",
				approved.Load,
				func(context.Context) codingtoolbox.ApprovalOutcome {
					calls.Add(1)
					approved.Store(true)
					return codingtoolbox.ApprovalOutcome{Shared: true}
				},
				nil,
			)
			assert.NoError(t, err)
		})
	}

	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), calls.Load(), "approveFn should be called exactly once")
}

func TestApprover_NonSharedRetries(t *testing.T) {
	a := codingtoolbox.NewApprover()

	var approveCalls atomic.Int32
	var retryCalls atomic.Int32
	leaderEntered := make(chan struct{})
	waiterQueued := make(chan struct{})

	var wg sync.WaitGroup

	// Leader goroutine: enters approveFn first and waits for waiter to coalesce.
	wg.Go(func() {
		err := a.Ensure(context.Background(), "k",
			func() bool { return false },
			func(context.Context) codingtoolbox.ApprovalOutcome {
				approveCalls.Add(1)
				close(leaderEntered) // signal that we're the leader
				<-waiterQueued       // wait for waiter to be under the lock
				return codingtoolbox.ApprovalOutcome{Shared: false}
			},
			func(context.Context) error {
				retryCalls.Add(1)
				return nil
			},
		)
		assert.NoError(t, err)
	})

	// Wait for leader to enter approveFn before starting waiter.
	<-leaderEntered

	// Waiter goroutine: will coalesce and then retry.
	// The second isApproved call happens under the Approver's lock, so signaling
	// there guarantees the waiter will see the pending entry before the leader
	// can finish and remove it.
	waiterIsApprovedCalls := 0
	wg.Go(func() {
		err := a.Ensure(context.Background(), "k",
			func() bool {
				waiterIsApprovedCalls++
				if waiterIsApprovedCalls == 2 {
					close(waiterQueued)
				}
				return false
			},
			func(context.Context) codingtoolbox.ApprovalOutcome {
				approveCalls.Add(1)
				return codingtoolbox.ApprovalOutcome{Shared: false}
			},
			func(context.Context) error {
				retryCalls.Add(1)
				return nil
			},
		)
		assert.NoError(t, err)
	})

	wg.Wait()

	assert.Equal(t, int32(1), approveCalls.Load(), "approveFn should be called once")
	assert.Equal(t, int32(1), retryCalls.Load(), "coalesced waiter should retry")
}

func TestApprover_ErrorPropagatedToWaiters(t *testing.T) {
	a := codingtoolbox.NewApprover()

	leaderEntered := make(chan struct{})
	waiterQueued := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)

	// Leader: enters approveFn and waits for waiter to coalesce.
	wg.Go(func() {
		errs[0] = a.Ensure(context.Background(), "k",
			func() bool { return false },
			func(context.Context) codingtoolbox.ApprovalOutcome {
				close(leaderEntered)
				<-waiterQueued
				return codingtoolbox.ApprovalOutcome{Err: fmt.Errorf("denied")}
			},
			nil,
		)
	})

	// Wait for leader to enter approveFn.
	<-leaderEntered

	// Waiter: will coalesce with the leader. Signal from the second isApproved
	// call (under the lock) to guarantee we see the pending entry.
	waiterIsApprovedCalls := 0
	wg.Go(func() {
		errs[1] = a.Ensure(context.Background(), "k",
			func() bool {
				waiterIsApprovedCalls++
				if waiterIsApprovedCalls == 2 {
					close(waiterQueued)
				}
				return false
			},
			func(context.Context) codingtoolbox.ApprovalOutcome {
				return codingtoolbox.ApprovalOutcome{Err: fmt.Errorf("should not be called")}
			},
			nil,
		)
	})

	wg.Wait()

	require.EqualError(t, errs[0], "denied")
	assert.EqualError(t, errs[1], "denied")
}
