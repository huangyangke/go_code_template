package gopool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPool_NonPositiveCap_DoesNotDeadlock(t *testing.T) {
	t.Run("new with zero cap still runs tasks", func(t *testing.T) {
		p := New("test", 0).(*pool)
		var ran atomic.Bool
		done := make(chan struct{})

		p.CtxGo(context.Background(), func(_ context.Context) {
			ran.Store(true)
			close(done)
		})

		select {
		case <-done:
			p.Wait()
			if !ran.Load() {
				t.Fatal("task did not run")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("task stayed queued forever when pool was created with cap=0")
		}
	})

	t.Run("set cap to zero still runs tasks", func(t *testing.T) {
		p := New("test", 1).(*pool)
		p.SetCap(0)
		var ran atomic.Bool
		done := make(chan struct{})

		p.CtxGo(context.Background(), func(_ context.Context) {
			ran.Store(true)
			close(done)
		})

		select {
		case <-done:
			p.Wait()
			if !ran.Load() {
				t.Fatal("task did not run")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("task stayed queued forever after SetCap(0)")
		}
	})
}

func TestPool_SubmissionDuringWorkerExit_DoesNotLeaveTaskQueued(t *testing.T) {
	for attempt := range 100 {
		p := New("test", 1).(*pool)
		firstStarted := make(chan struct{})
		releaseFirst := make(chan struct{})
		firstDone := make(chan struct{})
		secondRan := make(chan struct{})
		submitted := make(chan struct{})

		p.CtxGo(context.Background(), func(_ context.Context) {
			close(firstStarted)
			<-releaseFirst
			close(firstDone)
		})
		<-firstStarted

		// Hold the queue lock so the worker blocks on its next idle check before
		// the second submitter joins the wait queue behind it.
		p.taskLock.Lock()
		close(releaseFirst)
		<-firstDone
		time.Sleep(10 * time.Millisecond)

		go func() {
			p.CtxGo(context.Background(), func(_ context.Context) {
				close(secondRan)
			})
			close(submitted)
		}()

		time.Sleep(10 * time.Millisecond)
		p.taskLock.Unlock()
		<-submitted

		select {
		case <-secondRan:
			p.Wait()
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("attempt %d: second task remained queued without an active worker", attempt)
		}
	}
}
