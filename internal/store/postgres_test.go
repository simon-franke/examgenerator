package store

import (
	"math"
	"testing"
)

func TestEncodeVector(t *testing.T) {
	if got, want := encodeVector([]float32{1, -2.5, 0}), "[1,-2.5,0]"; got != want {
		t.Fatalf("encodeVector() = %q, want %q", got, want)
	}
}

func TestValidateVectorRejectsInvalidVectors(t *testing.T) {
	postgres := &Postgres{dimensions: 2}
	for _, test := range []struct {
		name   string
		vector []float32
	}{
		{name: "wrong dimensions", vector: []float32{1}},
		{name: "NaN", vector: []float32{float32(math.NaN()), 1}},
		{name: "positive infinity", vector: []float32{float32(math.Inf(1)), 1}},
		{name: "negative infinity", vector: []float32{float32(math.Inf(-1)), 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := postgres.validateVector(test.vector); err == nil {
				t.Fatal("expected invalid vector to fail validation")
			}
		})
	}
}
