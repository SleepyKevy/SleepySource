package main

import (
	"context"
	"log"
	"time"
)

func (a *App) startDetector() {
	ctx, cancel := context.WithCancel(context.Background())
	a.detectorCancel = cancel
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.detectorLoop(ctx)
	}()
}

func (a *App) detectorLoop(ctx context.Context) {
	runPlatformMediaDetector(ctx, a)
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (a *App) shutdown() {
	if a.cloudflare != nil {
		a.cloudflare.stop()
	}
	if a.chat != nil {
		a.chat.stopLiveChat()
	}
	if a.detectorCancel != nil {
		a.detectorCancel()
	}
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.server.Shutdown(ctx)
		cancel()
	}
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		log.Printf("shutdown timed out waiting for background workers")
	}
}
