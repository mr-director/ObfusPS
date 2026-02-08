package engine

import (
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	mathrand "math/rand"
	"time"
)

// InitRNG initializes the RNG. In deterministic mode (seeded=true) uses *seedOpt.
// In random mode, generates a seed (crypto/rand or time) and writes it to *seedOpt for reproducibility.
func InitRNG(seedOpt *int64, seeded bool) *mathrand.Rand {
	if seeded && seedOpt != nil {
		return mathrand.New(mathrand.NewSource(*seedOpt))
	}
	var seed int64
	var b [8]byte
	if _, err := crand.Read(b[:]); err == nil {
		seed = int64(0)
		for i := 0; i < 8; i++ {
			seed = (seed << 8) | int64(b[i])
		}
	} else {
		seed = time.Now().UnixNano()
	}
	if seedOpt != nil {
		*seedOpt = seed
	}
	return mathrand.New(mathrand.NewSource(seed))
}

func RandIdent(r *mathrand.Rand, n int) string {
	if n < 2 {
		n = 2
	}
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	var b []rune
	for i := 0; i < n; i++ {
		b = append(b, letters[r.Intn(len(letters))])
	}
	return string(b)
}

// Name families by role (buffer, stream, state, temp) for semantic consistency + fake business-like names.
var nameFamilies = map[string][]string{
	"buffer": {"buf", "data", "chunk", "blob", "payload", "Content", "Buffer", "Segment"},
	"stream": {"stm", "ms", "reader", "stream", "InputStream", "Reader", "Out"},
	"state":  {"s", "t", "idx", "n", "k", "State", "Index", "Pos"},
	"temp":   {"t1", "x", "tmp", "v", "acc", "Result", "Temp", "Hold"},
}

// RandVarByRole returns a variable name (without $) by role, mixing short/long and business style.
func RandVarByRole(r *mathrand.Rand, role string) string {
	list, ok := nameFamilies[role]
	if !ok || len(list) == 0 {
		return RandIdent(r, 5)
	}
	return list[r.Intn(len(list))]
}

func SumSha256(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func RandPerm(r *mathrand.Rand, a []string) {
	for i := range a {
		j := r.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
}

// ShuffleInts shuffles the int slice (Fisher-Yates).
func ShuffleInts(r *mathrand.Rand, a []int) {
	for i := len(a) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
}

func HexString(b []byte) string {
	return hex.EncodeToString(b)
}

// LCG constants (C standard rand).
const lcgMul = 1103515245
const lcgAdd = 12345

// DeriveKeyFromSeed produces 16 deterministic bytes from a seed (replicable in PS).
func DeriveKeyFromSeed(seed int64) []byte {
	return deriveKeyN(seed, 16)
}

// DeriveKey32FromSeed produces 32 bytes (double LCG derivation) for strengthened level 5.
func DeriveKey32FromSeed(seed int64) []byte {
	return deriveKeyN(seed, 32)
}

func deriveKeyN(seed int64, n int) []byte {
	key := make([]byte, n)
	s := uint32(seed & 0x7FFFFFFF)
	for i := 0; i < n; i++ {
		s = uint32((uint64(s)*lcgMul + lcgAdd) & 0x7FFFFFFF)
		key[i] = byte((s >> 16) & 0xFF)
	}
	return key
}
