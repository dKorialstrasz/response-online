package application

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMatchOwnerSerializesOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	owner := NewMatchOwner(16)
	runFinished := make(chan error, 1)
	go func() { runFinished <- owner.Run(ctx) }()

	var active int32
	var maximum int32
	var completed int32
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := owner.Submit(ctx, func(context.Context) error {
				current := atomic.AddInt32(&active, 1)
				for {
					observed := atomic.LoadInt32(&maximum)
					if current <= observed || atomic.CompareAndSwapInt32(&maximum, observed, current) {
						break
					}
				}
				atomic.AddInt32(&completed, 1)
				atomic.AddInt32(&active, -1)
				return nil
			})
			if err != nil {
				t.Errorf("Submit() error = %v", err)
			}
		}()
	}
	wait.Wait()
	cancel()
	<-runFinished

	if maximum != 1 {
		t.Fatalf("maximum concurrent operations = %d, want 1", maximum)
	}
	if completed != 32 {
		t.Fatalf("completed operations = %d, want 32", completed)
	}
}

func TestMatchOwnerRejectsNilOperation(t *testing.T) {
	owner := NewMatchOwner(1)
	if err := owner.Submit(context.Background(), nil); err == nil {
		t.Fatal("Submit(nil) returned nil error")
	}
}
