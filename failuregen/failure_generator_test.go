// Copyright 2022 Rubrik, Inc.

package failuregen_test

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rubrikinc/failure-test-utils/failuregen"
)

func TestFailureGeneratorFailsRandomly(t *testing.T) {
	g := failuregen.NewFailureGenerator()
	g.SetFailureProbability(0.20)

	failCount := 0
	for i := int32(0); i < failuregen.OneMillion; i++ {
		if g.FailMaybe() != nil {
			failCount++
		}
	}

	assert.InDelta(t, failuregen.OneMillion/5, failCount, 5000)
}

func cpuTimeNs() int64 {
	usage := new(syscall.Rusage)
	syscall.Getrusage(syscall.RUSAGE_SELF, usage)
	return usage.Utime.Nano() + usage.Stime.Nano()
}

func wallAndCPUTime(
	t *testing.T,
	proc func(int32),
) (time.Duration, time.Duration) {
	start := time.Now()
	cpuStart := cpuTimeNs()
	for i := int32(0); i < failuregen.OneMillion; i++ {
		proc(i)
	}
	elapsed := time.Now().Sub(start)
	cpuTime := cpuTimeNs() - cpuStart

	t.Logf("Actual elapsed time: %d micros", elapsed.Nanoseconds()/1000)
	t.Logf("Actual cpu time: %d micros", cpuTime/1000)

	return elapsed, time.Duration(cpuTime)
}

func TestFailureGeneratorDelaysRandomly(t *testing.T) {
	g := failuregen.NewFailureGenerator()
	g.SetDelayConfig(failuregen.DelayConfig{50, 0.2})

	wall, cpu := wallAndCPUTime(
		t,
		func(_ int32) { assert.NoError(t, g.FailMaybe()) })

	// 5s is mean due to uniform distribution with max = 50 micros
	assert.True(
		t,
		(wall-cpu).Nanoseconds() > 50*0.2*int64(failuregen.OneMillion)*1000/2)
}

func TestFailureGeneratorInducesExpectedDelay(t *testing.T) {
	g := failuregen.NewFailureGenerator()
	delayNanos := int64(0)
	g.(*failuregen.FailureGeneratorImpl).DelayFn = func(d time.Duration) {
		delayNanos += d.Nanoseconds()
	}
	g.SetDelayConfig(failuregen.DelayConfig{50, 0.2})

	wallAndCPUTime(
		t,
		func(_ int32) { assert.NoError(t, g.FailMaybe()) })

	// tolerance = 1s (20% of 5s)
	// 5s is mean due to uniform distribution with max = 50 micros
	assert.InDelta(
		t,
		50*0.2*float64(failuregen.OneMillion)*1000/2,
		float64(delayNanos),
		float64((1 * time.Second).Nanoseconds()))
}

func TestFailureGeneratorDoesNotFailOrDelayForProbabilityZero(t *testing.T) {
	g := failuregen.NewFailureGenerator()
	delayNanos := int64(0)
	g.(*failuregen.FailureGeneratorImpl).DelayFn = func(d time.Duration) {
		delayNanos += d.Nanoseconds()
	}

	// run with default configuration (no failures, no delay)
	failCount := 0
	wallAndCPUTime(
		t,
		func(_ int32) {
			if g.FailMaybe() != nil {
				failCount++
			}
		})
	assert.Zero(t, failCount)
	assert.Zero(t, delayNanos)

	// run with failure-probability explicitly set to zero
	failCount = 0
	g.SetFailureProbability(float32(0))
	g.SetDelayConfig(failuregen.DelayConfig{100000, 0.0})
	wallAndCPUTime(
		t,
		func(_ int32) {
			if g.FailMaybe() != nil {
				failCount++
			}
		})
	assert.Zero(t, failCount)
	assert.Zero(t, delayNanos)
}

func TestFailureGeneratorNeverSucceedsForFailProbabilityOne(t *testing.T) {
	g := failuregen.NewFailureGenerator()
	g.SetFailureProbability(1.0)
	successCount := 0
	for i := int32(0); i < failuregen.OneMillion; i++ {
		if g.FailMaybe() == nil {
			successCount++
		}
	}

	assert.Zero(t, successCount)
}

func TestInjectedFailureErrStackTraceShowsOnlyRelevantStack(t *testing.T) {
	g := failuregen.NewFailureGenerator()
	err := g.SetFailureProbability(1.0)
	require.NoError(t, err)
	err = g.FailMaybe()

	// https://stackoverflow.com/questions/32925344/why-is-there-a-fm-suffix-when-getting-a-functions-name-in-go
	method := reflect.ValueOf((*failuregen.FailureGeneratorImpl).FailMaybe).Pointer()
	methodName := runtime.FuncForPC(method).Name()

	stackTrace := fmt.Sprintf("%+v", err)

	require.Truef(t, strings.Contains(
		stackTrace,
		methodName,
	), "Stack trace:\n\n%s\n\n"+
		"should contain: %s", stackTrace, methodName)
}
