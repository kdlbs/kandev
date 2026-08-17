package clarification

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestWaitForResponseConfirmsDurableDeliveryBeforeReturning(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})
	confirmationStarted := make(chan struct{})
	releaseConfirmation := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseConfirmation) }) }
	t.Cleanup(release)
	waitDone := make(chan error, 1)
	respondDone := make(chan error, 1)
	go func() {
		_, err := s.WaitForResponse(context.Background(), id)
		waitDone <- err
	}()
	go func() {
		respondDone <- s.RespondWithDeliveryConfirmation(context.Background(), id, &Response{}, func() error {
			close(confirmationStarted)
			<-releaseConfirmation
			return nil
		})
	}()
	select {
	case <-confirmationStarted:
	case <-time.After(time.Second):
		t.Fatal("waiter did not start durable delivery confirmation")
	}
	if s.CancelRequest(id) {
		t.Fatal("resolved clarification was cancelled during delivery confirmation")
	}
	select {
	case err := <-waitDone:
		t.Fatalf("waiter returned before durable confirmation: %v", err)
	default:
	}
	select {
	case err := <-respondDone:
		t.Fatalf("responder returned before durable confirmation: %v", err)
	default:
	}
	release()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("WaitForResponse: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not return after durable confirmation")
	}
	select {
	case err := <-respondDone:
		if err != nil {
			t.Fatalf("RespondWithDeliveryConfirmation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("responder did not return after durable confirmation")
	}
}

func TestWaitForResponseRejectsFailedDeliveryConfirmation(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})
	wantErr := errors.New("database unavailable")
	waitDone := make(chan error, 1)
	go func() {
		_, err := s.WaitForResponse(context.Background(), id)
		waitDone <- err
	}()

	if err := s.RespondWithDeliveryConfirmation(
		context.Background(),
		id,
		&Response{},
		func() error { return wantErr },
	); !errors.Is(err, wantErr) {
		t.Fatalf("RespondWithDeliveryConfirmation error = %v, want %v", err, wantErr)
	}
	select {
	case err := <-waitDone:
		if !errors.Is(err, wantErr) {
			t.Fatalf("WaitForResponse error = %v, want %v", err, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not return after failed confirmation")
	}
}

func TestRespondWithDeliveryConfirmationAbandonsUnconsumedResponse(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	confirmed := false

	err := s.RespondWithDeliveryConfirmation(ctx, id, &Response{}, func() error {
		confirmed = true
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RespondWithDeliveryConfirmation error = %v, want deadline exceeded", err)
	}
	if confirmed {
		t.Fatal("delivery was confirmed without a consuming waiter")
	}
	if _, err := s.WaitForResponse(context.Background(), id); err == nil {
		t.Fatal("abandoned response remained consumable")
	}
}

func TestRespondWithDeliveryConfirmationUsesStoreTimeoutWithoutWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewStore(time.Second)
		id, _ := s.CreateRequest(&Request{SessionID: "s1"})
		respondDone := make(chan error, 1)
		go func() {
			respondDone <- s.RespondWithDeliveryConfirmation(
				context.Background(),
				id,
				&Response{},
				func() error { return nil },
			)
		}()

		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case err := <-respondDone:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("RespondWithDeliveryConfirmation error = %v, want store deadline", err)
			}
		default:
			waitDone := make(chan error, 1)
			go func() {
				_, err := s.WaitForResponse(context.Background(), id)
				waitDone <- err
			}()
			synctest.Wait()
			<-respondDone
			<-waitDone
			t.Fatal("RespondWithDeliveryConfirmation ignored the store timeout")
		}
	})
}

func TestRespondWithDeliveryConfirmationReturnsNotFoundWhenCancellationWinsAfterLookup(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})
	respondLoaded := make(chan struct{})
	releaseRespond := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRespond) }) }
	t.Cleanup(release)
	s.SetOnRespondLoaded(func(string) {
		close(respondLoaded)
		<-releaseRespond
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	respondDone := make(chan error, 1)
	confirmationCalled := make(chan struct{}, 1)
	go func() {
		respondDone <- s.RespondWithDeliveryConfirmation(ctx, id, &Response{}, func() error {
			confirmationCalled <- struct{}{}
			return nil
		})
	}()

	<-respondLoaded
	if !s.CancelRequest(id) {
		t.Fatal("CancelRequest returned false for known clarification")
	}
	cancel()
	release()

	select {
	case err := <-respondDone:
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("RespondWithDeliveryConfirmation error = %v, want %v", err, ErrNotFound)
		}
	case <-time.After(time.Second):
		t.Fatal("RespondWithDeliveryConfirmation blocked on a cancelled clarification")
	}
	select {
	case <-confirmationCalled:
		t.Fatal("cancelled clarification attempted delivery confirmation")
	default:
	}
}

func TestRespondWithDeliveryConfirmationFinishesStartedConfirmationAfterDeadline(t *testing.T) {
	s := NewStore(time.Minute)
	id, _ := s.CreateRequest(&Request{SessionID: "s1"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	confirmationStarted := make(chan struct{})
	releaseConfirmation := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseConfirmation) }) }
	t.Cleanup(release)
	waitDone := make(chan error, 1)
	respondDone := make(chan error, 1)
	go func() {
		_, err := s.WaitForResponse(context.Background(), id)
		waitDone <- err
	}()
	go func() {
		respondDone <- s.RespondWithDeliveryConfirmation(ctx, id, &Response{}, func() error {
			close(confirmationStarted)
			<-releaseConfirmation
			return nil
		})
	}()

	<-confirmationStarted
	<-ctx.Done()
	select {
	case err := <-respondDone:
		t.Fatalf("started confirmation was abandoned at deadline: %v", err)
	default:
	}
	release()
	if err := <-respondDone; err != nil {
		t.Fatalf("RespondWithDeliveryConfirmation: %v", err)
	}
	if err := <-waitDone; err != nil {
		t.Fatalf("WaitForResponse: %v", err)
	}
}

func TestRespondWithDeliveryConfirmationBoundsStartedConfirmationAfterDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewStore(time.Second)
		id, _ := s.CreateRequest(&Request{SessionID: "s1"})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		confirmationStarted := make(chan struct{})
		releaseConfirmation := make(chan struct{})
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(releaseConfirmation) }) }
		defer release()
		waitDone := make(chan error, 1)
		respondDone := make(chan error, 1)
		go func() {
			_, err := s.WaitForResponse(context.Background(), id)
			waitDone <- err
		}()
		go func() {
			respondDone <- s.RespondWithDeliveryConfirmation(ctx, id, &Response{}, func() error {
				close(confirmationStarted)
				<-releaseConfirmation
				return nil
			})
		}()

		synctest.Wait()
		select {
		case <-confirmationStarted:
		default:
			t.Fatal("waiter did not start delivery confirmation")
		}
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case err := <-respondDone:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("RespondWithDeliveryConfirmation error = %v, want store deadline", err)
			}
		default:
			t.Fatal("RespondWithDeliveryConfirmation blocked after its finish timeout")
		}
		select {
		case err := <-waitDone:
			t.Fatalf("WaitForResponse returned before confirmation finished: %v", err)
		default:
		}

		release()
		synctest.Wait()
		if err := <-waitDone; err != nil {
			t.Fatalf("WaitForResponse: %v", err)
		}
	})
}

func TestRespondWithDeliveryConfirmationCapsStartedConfirmationBelowStoreTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewStore(10 * time.Minute)
		id, _ := s.CreateRequest(&Request{SessionID: "s1"})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		confirmationStarted := make(chan struct{})
		releaseConfirmation := make(chan struct{})
		defer close(releaseConfirmation)
		go func() {
			_, _ = s.WaitForResponse(context.Background(), id)
		}()
		respondDone := make(chan error, 1)
		go func() {
			respondDone <- s.RespondWithDeliveryConfirmation(ctx, id, &Response{}, func() error {
				close(confirmationStarted)
				<-releaseConfirmation
				return nil
			})
		}()

		synctest.Wait()
		select {
		case <-confirmationStarted:
		default:
			t.Fatal("waiter did not start delivery confirmation")
		}
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		select {
		case err := <-respondDone:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("RespondWithDeliveryConfirmation error = %v, want capped deadline", err)
			}
		default:
			t.Fatal("RespondWithDeliveryConfirmation used the longer store timeout")
		}
	})
}
