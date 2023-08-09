package random

import (
	"math/rand"
	"time"
)

// NewTimeSeededRand creates *rand.Rand seeded with current time
// and safe for concurrent use
func NewTimeSeededRand() *rand.Rand {
	return rand.New(
		NewConcurrentRandomSource(
			rand.NewSource(
				time.Now().UnixNano(),
			),
		),
	)
}
