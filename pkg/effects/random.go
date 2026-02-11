// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package effects

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"
)

// RandomResult represents the recorded random values.
type RandomResult struct {
	Source   string `json:"source"`
	Values   []byte `json:"values"`
	Length   int    `json:"length"`
	SeedUsed int64  `json:"seed_used,omitempty"`
}

// ExecuteRandomInt63 generates a random int64.
func ExecuteRandomInt63(ctx context.Context, sys System, source string) (int64, error) {
	result := RandomResult{
		Source: source,
		Length: 8,
		Values: make([]byte, 8),
	}

	// Use a deterministic seed based on source + system time for first run
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))

	val := rng.Int63()
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte((val >> (i * 8)) & 0xff)
	}
	result.Values = b
	result.SeedUsed = seed

	effect := NewEffect(KindRandom, result).
		WithIdempotencyKey("random:" + source + ":int63").
		WithDescription("random.int63")

	res, err := sys.Execute(ctx, effect)
	if err != nil {
		return 0, err
	}

	if res.Cached && res.Data != nil {
		randRes := res.Data.(RandomResult)
		// Extract int64 from cached bytes
		var val int64
		for i, b := range randRes.Values {
			val |= int64(b) << (i * 8)
		}
		return val, nil
	}

	// Record the generated value
	res.Data = result
	return val, nil
}

// ExecuteRandomBytes generates random bytes of the specified length.
func ExecuteRandomBytes(ctx context.Context, sys System, source string, length int) ([]byte, error) {
	result := RandomResult{
		Source: source,
		Length: length,
		Values: make([]byte, length),
	}

	// Use a deterministic seed
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	rng.Read(result.Values)

	effect := NewEffect(KindRandom, result).
		WithIdempotencyKey("random:" + source + ":bytes:" + fmt.Sprintf("%d", length)).
		WithDescription("random.bytes")

	res, err := sys.Execute(ctx, effect)
	if err != nil {
		return nil, err
	}

	if res.Cached && res.Data != nil {
		return res.Data.(RandomResult).Values, nil
	}

	res.Data = result
	return result.Values, nil
}

// ExecuteRandomIntn generates a random integer in [0, n).
func ExecuteRandomIntn(ctx context.Context, sys System, source string, n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}

	val, err := ExecuteRandomInt63(ctx, sys, source+":intn:"+fmt.Sprintf("%d", n))
	if err != nil {
		return 0, err
	}
	return int(val % int64(n)), nil
}

// RandomEffect creates an effect for random value generation.
func RandomEffect(source string, values []byte) Effect {
	result := RandomResult{
		Source: source,
		Values: values,
		Length: len(values),
	}
	return NewEffect(KindRandom, result).
		WithIdempotencyKey("random:" + source + ":" + hex.EncodeToString(values)).
		WithDescription("random.source")
}

// RecordRandomToRecorder records a random effect using the EventRecorder.
func RecordRandomToRecorder(ctx context.Context, recorder EventRecorder, effectID string, source string, values []byte) error {
	if recorder == nil {
		return ErrNoRecorder
	}
	return recorder.RecordRandom(ctx, effectID, source, values)
}