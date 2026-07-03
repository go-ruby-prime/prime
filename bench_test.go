// Copyright (c) the go-ruby-prime/prime authors
//
// SPDX-License-Identifier: BSD-3-Clause

package prime

import (
	"math/big"
	"testing"
)

// The generator benchmarks track the enumeration path that rbgo drives for
// Prime.each / Prime.first / Prime.take. Before the memoized incremental sieve
// landed, each enumeration re-ran a Baillie–PSW primality test per candidate
// (hundreds of big.Int temporaries apiece), so enumerating the 100 primes up to
// 541 cost ~125000 ns/op; the shared growing sieve brings the same work to
// ~2000 ns/op (Apple M4 Max, go1.26.4) — a ~60x reduction that closes the
// reported ~15x gap versus MRI's own memoizing Prime singleton.

// BenchmarkEachTo541 mirrors rbgo's Prime.each(541) — the 100 primes <= 541.
func BenchmarkEachTo541(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Each(541, func(p *big.Int) bool { return true })
	}
}

// BenchmarkEachTo10k enumerates the 1229 primes <= 10^4.
func BenchmarkEachTo10k(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Each(10000, func(p *big.Int) bool { return true })
	}
}

// BenchmarkTake100 is the unbounded-generator path (Prime.take(100)): 100 primes
// pulled through the EachPrime cursor, the exact shape rbgo binds Prime.each to.
func BenchmarkTake100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Take(100)
	}
}

// BenchmarkEachPrimeCursor drives the stateful generator directly for 100 pulls.
func BenchmarkEachPrimeCursor(b *testing.B) {
	for i := 0; i < b.N; i++ {
		it := EachPrime()
		for j := 0; j < 100; j++ {
			it()
		}
	}
}

// BenchmarkIsPrimePrime tracks the primality fast path on a 30-bit prime — the
// allocation-free uint64 trial-division-plus-Miller–Rabin route (rbgo's
// Integer#prime? / Prime.prime?). Before the word-size path it ran big.Int
// Baillie–PSW with hundreds of temporaries per call.
func BenchmarkIsPrimePrime(b *testing.B) {
	p := big.NewInt(982451653)
	for i := 0; i < b.N; i++ {
		sinkBool = IsPrime(p)
	}
}

// BenchmarkIsPrimeComposite tracks the same path on an odd semiprime with no
// small factor, so it reaches Miller–Rabin and is rejected there.
func BenchmarkIsPrimeComposite(b *testing.B) {
	c := new(big.Int).Mul(big.NewInt(982451651), big.NewInt(982451653))
	for i := 0; i < b.N; i++ {
		sinkBool = IsPrime(c)
	}
}

// sinkBool defeats dead-code elimination of the primality benchmarks.
var sinkBool bool
