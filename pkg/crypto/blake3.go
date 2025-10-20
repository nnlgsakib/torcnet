package crypto

import (
	"github.com/zeebo/blake3"
)

// Hash computes the BLAKE3 hash of the input data
func Hash(data []byte) []byte {
	hasher := blake3.New()
	hasher.Write(data)
	return hasher.Sum(nil)
}

// HashMultiple computes the BLAKE3 hash of multiple inputs
func HashMultiple(inputs ...[]byte) []byte {
	hasher := blake3.New()
	for _, input := range inputs {
		hasher.Write(input)
	}
	return hasher.Sum(nil)
}