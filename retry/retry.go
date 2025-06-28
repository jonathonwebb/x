package retry

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

const (
	maxDuration time.Duration = math.MaxInt64
	maxInt      int           = math.MaxInt
)

type RetryOptions struct {
	MaxTries      int
	Delay         time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	Jitter        float64
}

func DefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxTries:      maxInt,
		Delay:         0,
		MaxDelay:      maxDuration,
		BackoffFactor: 1.0,
		Jitter:        0.0,
	}
}

type Option func(*RetryOptions)

func WithMaxTries(tries int) Option {
	return func(ro *RetryOptions) {
		ro.MaxTries = tries
	}
}

func WithDelay(delay time.Duration) Option {
	return func(ro *RetryOptions) {
		ro.Delay = delay
	}
}

func WithMaxDelay(delay time.Duration) Option {
	return func(ro *RetryOptions) {
		ro.MaxDelay = delay
	}
}

func WithBackoffFactor(factor float64) Option {
	return func(ro *RetryOptions) {
		ro.BackoffFactor = factor
	}
}

func WithJitter(jitter float64) Option {
	return func(ro *RetryOptions) {
		ro.Jitter = jitter
	}
}

func retry[T any](ctx context.Context, fn func(ctx context.Context) (T, error), opts ...Option) (T, error) {
	options := DefaultRetryOptions()
	for _, opt := range opts {
		opt(&options)
	}

	var lastErr error
	var zero T
	currentDelay := options.Delay

	for i := 1; i <= options.MaxTries; i++ {
		res, err := fn(ctx)

		if err == nil {
			return res, nil
		}

		lastErr = err

		if i == options.MaxTries {
			break
		}

		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		sleepDuration := min(currentDelay, options.MaxDelay)
		if options.Jitter > 0 {
			jitterAmount := time.Duration(options.Jitter * float64(sleepDuration))
			sleepDuration += time.Duration(rand.Float64()*float64(jitterAmount)) - (jitterAmount / 2)
		}

		timer := time.NewTimer(sleepDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
			// proceed
		}

		currentDelay = time.Duration(float64(currentDelay) * options.BackoffFactor)
	}

	return zero, lastErr
}

func Retry(fn func(ctx context.Context) error, opts ...Option) error {
	_, err := retry(context.Background(), func(ctx context.Context) (any, error) { return nil, fn(ctx) }, opts...)
	return err
}

func RetryContext(ctx context.Context, fn func(ctx context.Context) error, opts ...Option) error {
	_, err := retry(ctx, func(ctx context.Context) (any, error) { return nil, fn(ctx) }, opts...)
	return err
}

func RetryValue[T any](fn func(ctx context.Context) (T, error), opts ...Option) (T, error) {
	return retry(context.Background(), fn, opts...)
}

func RetryValueContext[T any](ctx context.Context, fn func(ctx context.Context) (T, error), opts ...Option) (T, error) {
	return retry(ctx, fn, opts...)
}
