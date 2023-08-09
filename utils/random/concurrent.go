package random

import (
	"math/rand"
	"sync"
)

type concurrentRandomSource struct {
	src rand.Source
	mux sync.Mutex
}

// NewConcurrentRandomSource constructs ConcurrentRandomSource, wrapping
// existing rand.Source/rand.Source64
func NewConcurrentRandomSource(src rand.Source) rand.Source {
	src64, ok := src.(rand.Source64)
	if ok {
		return &concurrentRandomSource64{
			src: src64,
		}
	}
	return &concurrentRandomSource{
		src: src,
	}
}

// Seed is a part of rand.Source interface
func (s *concurrentRandomSource) Seed(seed int64) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.src.Seed(seed)
}

// Int63 is a part of rand.Source interface
func (s *concurrentRandomSource) Int63() int64 {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.src.Int63()
}

type concurrentRandomSource64 struct {
	src rand.Source64
	mux sync.Mutex
}

// Seed is a part of rand.Source interface
func (s *concurrentRandomSource64) Seed(seed int64) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.src.Seed(seed)
}

// Int63 is a part of rand.Source interface
func (s *concurrentRandomSource64) Int63() int64 {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.src.Int63()
}

// Uint64 is a part of rand.Source64 interface
func (s *concurrentRandomSource64) Uint64() uint64 {
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.src.Uint64()
}
