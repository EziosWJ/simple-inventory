package directdelivery

import (
	"context"
	"time"
)

type effectiveTimeKey struct{}

// WithEffectiveTime carries a timestamp selected after all transaction locks.
func WithEffectiveTime(ctx context.Context, stamp time.Time) context.Context {
	return context.WithValue(ctx, effectiveTimeKey{}, stamp)
}
func EffectiveTime(ctx context.Context) time.Time {
	if stamp, ok := ctx.Value(effectiveTimeKey{}).(time.Time); ok {
		return stamp
	}
	return time.Now().UTC()
}
