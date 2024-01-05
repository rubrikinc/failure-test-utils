package randutil

import (
	"sync"
	"testing"
	"time"
)

// OneMillion is a convenient constant for 1M
const OneMillion = int32(1000000)

// Should not panic
func TestConcurrentRandomNumberGeneration(t *testing.T) {
	rng := NewLockedRandGen(time.Now().Unix())
	concurrency := 300
	wg := &sync.WaitGroup{}
	startCh := make(chan struct{})
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			<-startCh
			for j := 0; j < 50000; j++ {
				_ = rng.Int31n(OneMillion)
				time.Sleep(1 * time.Microsecond)
			}
		}()
	}
	close(startCh)
	wg.Wait()
}

// TestConcurrentRandomNumberGenerationMightPanic just asserts that without
// locking around random generator there is possibility of running into panic,
// and we had issues found in internal testing. E.g. CDM-401909
func TestConcurrentRandomNumberGenerationMightPanic(t *testing.T) {
	rng := NewLockedRandGen(time.Now().Unix())
	concurrency := 300
	wg := &sync.WaitGroup{}
	startCh := make(chan struct{})
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer func() {
				if err := recover(); err != nil {
					t.Logf("Function raised panic: %s\nIgnoring", err)
				}
			}()
			defer wg.Done()
			<-startCh
			for j := 0; j < 50000; j++ {
				_ = rng.Int31nWOLockForTest(OneMillion)
				time.Sleep(1 * time.Microsecond)
			}
		}()
	}
	close(startCh)
	wg.Wait()
}
