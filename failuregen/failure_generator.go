// Copyright 2022 Rubrik, Inc.

package failuregen

import (
	"math/rand"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/atomic"
)

var errInjectedFailure = errors.New("Injected failure")

// OneMillion is a convenient constant for 1M
const OneMillion = int32(1000000)

// DelayConfig to be used for injecting delays to make races likely
type DelayConfig struct {
	// MaxDelayMicros is maximum possible delay at a failure point
	MaxDelayMicros int32
	// DelayProbability is probability of delay
	DelayProbability float32
}

// FailureGenerator generates artificial failures and delays with
// configured probability / magnitude.
type FailureGenerator interface {
	SetDelayConfig(c DelayConfig) error
	SetFailureProbability(p float32) error
	FailMaybe() error
}

type delayFn func(time.Duration)

type FailureGeneratorImpl struct {
	failurePpm     atomic.Int32
	delayPpm       atomic.Int32
	maxDelayMicros atomic.Int32
	DelayFn        delayFn
}

// NewFailureGenerator creates a new failure-generator
func NewFailureGenerator() FailureGenerator {
	return &FailureGeneratorImpl{DelayFn: time.Sleep}
}

// ppm => parts per million
// ppm:10^6 :: percent:100 :: probability:1
func ppm(p float32) (int32, error) {
	if p < 0 || p > 1.0 {
		return 0, errors.Errorf("Invalid probability %f not in [0.0, 1.0]", p)
	}
	return int32(p * float32(OneMillion)), nil
}

// SetDelayConfig sets configuration for injecting artificial delay
func (fg *FailureGeneratorImpl) SetDelayConfig(c DelayConfig) error {
	delayPpm, err := ppm(c.DelayProbability)
	if err != nil {
		return errors.Wrapf(err, "Couldn't compute delay-ppm")
	}
	if c.MaxDelayMicros < 0 {
		return errors.Errorf("Invalid delay of %d microseconds", c.MaxDelayMicros)
	}
	fg.delayPpm.Store(delayPpm)
	fg.maxDelayMicros.Store(c.MaxDelayMicros)
	return nil
}

// SetFailureProbability sets the desired artificial failure probability
func (fg *FailureGeneratorImpl) SetFailureProbability(p float32) error {
	failurePpm, err := ppm(p)
	if err != nil {
		return errors.Wrapf(err, "Couldn't compute failure-ppm")
	}
	fg.failurePpm.Store(failurePpm)
	return nil
}

// FailMaybe returns an artificial error with configured probability
func (fg *FailureGeneratorImpl) FailMaybe() error {
	if rand.Int31n(OneMillion) < fg.delayPpm.Load() {
		fg.DelayFn(
			time.Duration(rand.Int31n(fg.maxDelayMicros.Load())) * time.Microsecond)
	}
	if rand.Int31n(OneMillion) < fg.failurePpm.Load() {
		return errInjectedFailure
	}
	return nil
}
