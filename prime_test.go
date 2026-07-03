// Copyright (c) the go-ruby-prime/prime authors
//
// SPDX-License-Identifier: BSD-3-Clause

package prime

import (
	"math/big"
	"math/rand"
	"reflect"
	"testing"
)

// TestIsPrimeAgainstOracle is the correctness backstop for the optimized paths:
// it checks IsPrime exhaustively over a small range against a plain sieve, then
// over a wide random sample of the whole uint64 range and a band just above
// 2^64 against math/big's Baillie–PSW (which is proven exact below 2^64). This
// pins the word-size fast path and the arbitrary-precision path to an
// independent reference across their full domains.
func TestIsPrimeAgainstOracle(t *testing.T) {
	const N = 200000
	composite := make([]bool, N+1)
	for i := 2; i*i <= N; i++ {
		if !composite[i] {
			for j := i * i; j <= N; j += i {
				composite[j] = true
			}
		}
	}
	for n := 0; n <= N; n++ {
		want := n >= 2 && !composite[n]
		if got := IsPrime(big.NewInt(int64(n))); got != want {
			t.Fatalf("IsPrime(%d) = %v, want %v (sieve)", n, got, want)
		}
	}
	// Wide random sample of the uint64 fast path against the exact big oracle.
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		v := new(big.Int).SetUint64(r.Uint64())
		if got, want := IsPrime(v), v.ProbablyPrime(0); got != want {
			t.Fatalf("IsPrime(%s) = %v, want %v (big oracle)", v, got, want)
		}
	}
	// A band just above 2^64 drives the arbitrary-precision path against the oracle.
	base := new(big.Int).Lsh(big.NewInt(1), 64)
	for i := int64(0); i < 5000; i++ {
		v := new(big.Int).Add(base, big.NewInt(i))
		if got, want := IsPrime(v), v.ProbablyPrime(20); got != want {
			t.Fatalf("IsPrime(2^64+%d) = %v, want %v (big oracle)", i, got, want)
		}
	}
}

// bi parses a base-10 string into a *big.Int (test helper).
func bi(t *testing.T, s string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad int literal %q", s)
	}
	return v
}

// pairsToInts flattens [][2]*big.Int to [][2]int64 for easy comparison.
func pairsToInts(ps [][2]*big.Int) [][2]int64 {
	out := make([][2]int64, len(ps))
	for i, p := range ps {
		out[i] = [2]int64{p[0].Int64(), p[1].Int64()}
	}
	return out
}

// intsOf flattens []*big.Int to []int64.
func intsOf(ps []*big.Int) []int64 {
	out := make([]int64, len(ps))
	for i, p := range ps {
		out[i] = p.Int64()
	}
	return out
}

func TestIsPrimeSmall(t *testing.T) {
	// Golden table matching MRI Prime.prime? for 0..30 plus negatives.
	want := map[int64]bool{
		-7: false, -2: false, -1: false, 0: false, 1: false,
		2: true, 3: true, 4: false, 5: true, 6: false, 7: true,
		8: false, 9: false, 10: false, 11: true, 12: false, 13: true,
		17: true, 19: true, 23: true, 25: false, 29: true, 30: false,
	}
	for n, w := range want {
		if got := IsPrime(big.NewInt(n)); got != w {
			t.Errorf("IsPrime(%d) = %v, want %v", n, got, w)
		}
	}
}

func TestIsPrimeNil(t *testing.T) {
	if IsPrime(nil) {
		t.Error("IsPrime(nil) should be false")
	}
}

func TestIsPrimeCarmichael(t *testing.T) {
	// Carmichael numbers: composite but Fermat-pseudoprime to every coprime base.
	for _, c := range []int64{561, 1105, 1729, 2465, 2821, 6601, 8911, 41041, 825265} {
		if IsPrime(big.NewInt(c)) {
			t.Errorf("IsPrime(%d) Carmichael should be composite", c)
		}
	}
}

func TestIsPrimePseudoprimes(t *testing.T) {
	// Strong base-2 pseudoprimes and a strong pseudoprime to bases 2,3,5,7 — all
	// composite, all rejected by Baillie–PSW.
	for _, c := range []int64{2047, 3277, 4033, 4681, 8321, 3215031751} {
		if IsPrime(big.NewInt(c)) {
			t.Errorf("IsPrime(%d) pseudoprime should be composite", c)
		}
	}
}

func TestIsPrimeLarge(t *testing.T) {
	cases := []struct {
		n    string
		want bool
	}{
		{"1000000007", true},           // prime
		{"1000000009", true},           // prime
		{"1000000016000000063", false}, // composite cofactor
		{"2305843009213693951", true},  // Mersenne 2^61-1, prime
		{"1000000007000000063", false},
		{"9223372036854775783", true},                     // largest prime < 2^63
		{"9223372036854775807", false},                    // 2^63-1 = 7^2 * 73 * 127 * 337 * 92737 * 649657
		{"18446744073709551557", true},                    // largest prime < 2^64
		{"170141183460469231731687303715884105727", true}, // Mersenne 2^127-1, prime
	}
	for _, c := range cases {
		if got := IsPrime(bi(t, c.n)); got != c.want {
			t.Errorf("IsPrime(%s) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestIsPrimeEvenLarge(t *testing.T) {
	if IsPrime(bi(t, "100000000000000000000")) {
		t.Error("large even number is composite")
	}
}

func TestTakeAndFirst(t *testing.T) {
	want := []int64{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53, 59, 61, 67, 71}
	if got := intsOf(Take(20)); !reflect.DeepEqual(got, want) {
		t.Errorf("Take(20) = %v, want %v", got, want)
	}
	if got := intsOf(First(5)); !reflect.DeepEqual(got, []int64{2, 3, 5, 7, 11}) {
		t.Errorf("First(5) = %v", got)
	}
}

func TestTakeNonPositive(t *testing.T) {
	if got := Take(0); len(got) != 0 {
		t.Errorf("Take(0) = %v, want empty", got)
	}
	if got := First(-3); len(got) != 0 {
		t.Errorf("First(-3) = %v, want empty", got)
	}
}

func TestEach(t *testing.T) {
	collect := func(ub int64) []int64 {
		var out []int64
		Each(ub, func(p *big.Int) bool { out = append(out, p.Int64()); return true })
		return out
	}
	if got := collect(11); !reflect.DeepEqual(got, []int64{2, 3, 5, 7, 11}) {
		t.Errorf("Each(11) = %v", got)
	}
	if got := collect(10); !reflect.DeepEqual(got, []int64{2, 3, 5, 7}) {
		t.Errorf("Each(10) = %v", got)
	}
	if got := collect(2); !reflect.DeepEqual(got, []int64{2}) {
		t.Errorf("Each(2) = %v", got)
	}
	if got := collect(1); got != nil {
		t.Errorf("Each(1) = %v, want nil", got)
	}
	if got := collect(0); got != nil {
		t.Errorf("Each(0) = %v, want nil", got)
	}
}

func TestEachEarlyStop(t *testing.T) {
	var out []int64
	Each(100, func(p *big.Int) bool {
		out = append(out, p.Int64())
		return len(out) < 3 // stop after 3
	})
	if !reflect.DeepEqual(out, []int64{2, 3, 5}) {
		t.Errorf("Each early-stop = %v, want [2 3 5]", out)
	}
}

func TestPrimeDivision(t *testing.T) {
	cases := []struct {
		n    int64
		want [][2]int64
	}{
		{12, [][2]int64{{2, 2}, {3, 1}}},
		{-12, [][2]int64{{-1, 1}, {2, 2}, {3, 1}}},
		{360, [][2]int64{{2, 3}, {3, 2}, {5, 1}}},
		{100, [][2]int64{{2, 2}, {5, 2}}},
		{1, nil},
		{-1, [][2]int64{{-1, 1}}},
		{2, [][2]int64{{2, 1}}},
		{-2, [][2]int64{{-1, 1}, {2, 1}}},
		{7919, [][2]int64{{7919, 1}}},
		{-7, [][2]int64{{-1, 1}, {7, 1}}},
	}
	for _, c := range cases {
		got := pairsToInts(PrimeDivision(big.NewInt(c.n)))
		if c.want == nil {
			if len(got) != 0 {
				t.Errorf("PrimeDivision(%d) = %v, want []", c.n, got)
			}
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("PrimeDivision(%d) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestPrimeDivisionLarge(t *testing.T) {
	got := pairsToInts(PrimeDivision(bi(t, "1234567890123456789")))
	want := [][2]int64{{3, 2}, {101, 1}, {3541, 1}, {3607, 1}, {3803, 1}, {27961, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PrimeDivision(1234567890123456789) = %v, want %v", got, want)
	}
	// Mersenne prime stays whole.
	m := pairsToInts(PrimeDivision(bi(t, "2305843009213693951")))
	if !reflect.DeepEqual(m, [][2]int64{{2305843009213693951, 1}}) {
		t.Errorf("PrimeDivision(mersenne) = %v", m)
	}
}

func TestPrimeDivisionSemiprimeBig(t *testing.T) {
	// Product of two large primes — exercises Pollard's rho on a hard semiprime.
	n := new(big.Int).Mul(bi(t, "1000000007"), bi(t, "1000000009"))
	got := pairsToInts(PrimeDivision(n))
	want := [][2]int64{{1000000007, 1}, {1000000009, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PrimeDivision(semiprime) = %v, want %v", got, want)
	}
}

func TestPrimeDivisionPrimePower(t *testing.T) {
	// 1009^3, a large prime cube — Pollard's rho must split repeated factors.
	n := new(big.Int).Exp(big.NewInt(1009), big.NewInt(3), nil)
	got := pairsToInts(PrimeDivision(n))
	if !reflect.DeepEqual(got, [][2]int64{{1009, 3}}) {
		t.Errorf("PrimeDivision(1009^3) = %v", got)
	}
}

func TestPrimeDivisionEvenCofactor(t *testing.T) {
	// 2 * largeprime: cofactor after small-prime stripping is even -> rho even branch.
	n := new(big.Int).Mul(big.NewInt(2), bi(t, "9223372036854775783"))
	got := pairsToInts(PrimeDivision(n))
	want := [][2]int64{{2, 1}, {9223372036854775783, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PrimeDivision(2*prime) = %v, want %v", got, want)
	}
}

func TestPrimeDivisionZeroPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("PrimeDivision(0) should panic")
		}
		if _, ok := r.(ZeroError); !ok {
			t.Fatalf("panic value = %T (%v), want ZeroError", r, r)
		}
	}()
	PrimeDivision(big.NewInt(0))
}

func TestPrimeDivisionErr(t *testing.T) {
	if _, err := PrimeDivisionErr(big.NewInt(0)); err == nil {
		t.Fatal("PrimeDivisionErr(0) should error")
	} else if err.Error() != "ZeroDivisionError" {
		t.Errorf("err = %q", err.Error())
	}
	if _, err := PrimeDivisionErr(nil); err == nil {
		t.Fatal("PrimeDivisionErr(nil) should error")
	}
	if _, err := PrimeDivisionErr(big.NewInt(12)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIntRoundTrip(t *testing.T) {
	for _, n := range []int64{12, -12, 1, -1, 360, 7919, -7, 100} {
		ps := PrimeDivision(big.NewInt(n))
		if got := Int(ps); got.Cmp(big.NewInt(n)) != 0 {
			t.Errorf("Int(PrimeDivision(%d)) = %v, want %d", n, got, n)
		}
	}
}

func TestIntExplicit(t *testing.T) {
	// Mirrors Prime.int_from_prime_division samples.
	v := Int([][2]*big.Int{{big.NewInt(2), big.NewInt(2)}, {big.NewInt(3), big.NewInt(1)}})
	if v.Cmp(big.NewInt(12)) != 0 {
		t.Errorf("Int([[2,2],[3,1]]) = %v, want 12", v)
	}
	v = Int([][2]*big.Int{{bigNegOne, big.NewInt(1)}, {big.NewInt(2), big.NewInt(2)}, {big.NewInt(3), big.NewInt(1)}})
	if v.Cmp(big.NewInt(-12)) != 0 {
		t.Errorf("Int(neg) = %v, want -12", v)
	}
	if got := Int(nil); got.Cmp(bigOne) != 0 {
		t.Errorf("Int(nil) = %v, want 1", got)
	}
}

func TestIntLargeRoundTrip(t *testing.T) {
	n := bi(t, "1234567890123456789")
	if got := Int(PrimeDivision(n)); got.Cmp(n) != 0 {
		t.Errorf("Int round-trip large = %v, want %v", got, n)
	}
}

func TestNextPrev(t *testing.T) {
	cases := []struct {
		n          int64
		next, prev int64 // prev == -1 means nil
	}{
		{1, 2, -1},
		{2, 3, -1},
		{3, 5, 2},
		{13, 17, 11},
		{14, 17, 13},
		{0, 2, -1},
		{-5, 2, -1},
		{7919, 7927, 7907},
	}
	for _, c := range cases {
		if got := Next(big.NewInt(c.n)).Int64(); got != c.next {
			t.Errorf("Next(%d) = %d, want %d", c.n, got, c.next)
		}
		got := Prev(big.NewInt(c.n))
		if c.prev == -1 {
			if got != nil {
				t.Errorf("Prev(%d) = %v, want nil", c.n, got)
			}
		} else if got == nil || got.Int64() != c.prev {
			t.Errorf("Prev(%d) = %v, want %d", c.n, got, c.prev)
		}
	}
}

func TestNextPrevLarge(t *testing.T) {
	// Around a known large prime gap.
	n := bi(t, "1000000007")
	if Next(n).Cmp(bi(t, "1000000009")) != 0 {
		t.Errorf("Next(1000000007) = %v", Next(n))
	}
	if Prev(n).Cmp(bi(t, "999999937")) != 0 {
		t.Errorf("Prev(1000000007) = %v", Prev(n))
	}
}

func TestEachPrimeGenerator(t *testing.T) {
	it := EachPrime()
	var got []int64
	for i := 0; i < 10; i++ {
		got = append(got, it().Int64())
	}
	want := []int64{2, 3, 5, 7, 11, 13, 17, 19, 23, 29}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EachPrime first 10 = %v, want %v", got, want)
	}
}

func TestZeroErrorMessage(t *testing.T) {
	if (ZeroError{}).Error() != "ZeroDivisionError" {
		t.Error("ZeroError message mismatch")
	}
}

// TestInternalHelpers exercises the low-level helpers directly so the
// deterministic suite alone reaches 100% even on the no-ruby lanes.
func TestInternalHelpers(t *testing.T) {
	if sievePrimes(1) != nil {
		t.Error("sievePrimes(1) should be nil")
	}
	if !isPerfectSquare(big.NewInt(144)) || isPerfectSquare(big.NewInt(145)) {
		t.Error("isPerfectSquare wrong")
	}
	if isPerfectSquare(big.NewInt(-4)) {
		t.Error("isPerfectSquare(neg) should be false")
	}
	// jacobi(1, n) == 1; jacobi where gcd != 1 -> 0.
	if jacobi(big.NewInt(1), big.NewInt(9)) != 1 {
		t.Error("jacobi(1,9) != 1")
	}
	if jacobi(big.NewInt(3), big.NewInt(9)) != 0 {
		t.Error("jacobi(3,9) != 0")
	}
	if jacobi(big.NewInt(2), big.NewInt(7)) != 1 {
		t.Error("jacobi(2,7) != 1")
	}
	// jacobi(2,5): the factor-of-2 reduction with n ≡ 5 (mod 8) flips the sign.
	if jacobi(big.NewInt(2), big.NewInt(5)) != -1 {
		t.Error("jacobi(2,5) != -1")
	}
	// jacobi(3,7): both a and n ≡ 3 (mod 4), exercising the reciprocity flip.
	if jacobi(big.NewInt(3), big.NewInt(7)) != -1 {
		t.Error("jacobi(3,7) != -1")
	}
	// pollardRho on an even number returns 2 immediately.
	if pollardRho(big.NewInt(8)).Int64() != 2 {
		t.Error("pollardRho(8) != 2")
	}
	// pollardRho on 21: the first polynomial (c=1) closes its cycle without a
	// factor (the diff==0 break and gcd==1 path), forcing the retry with c=2,
	// which then yields 3 — exercising both the cycle-close break and the retry.
	if d := pollardRho(big.NewInt(21)); d.Int64() != 3 && d.Int64() != 7 {
		t.Errorf("pollardRho(21) = %v, want a proper factor", d)
	}
}

// TestMillerRabinUint64Direct drives the deterministic word-size Miller–Rabin
// on composites that IsPrime's small-factor trial division would otherwise
// short-circuit, so every branch (the immediate-composite reject, the in-loop
// n-1 break, the witness-equals-n skip and the prime accept) is exercised.
func TestMillerRabinUint64Direct(t *testing.T) {
	// Base-2 strong pseudoprime: the first witness passes (x==1 → continue), a
	// later witness settles it immediately (no square reaches n-1).
	if millerRabinUint64(2047) {
		t.Error("millerRabinUint64(2047) should be composite")
	}
	// Composite whose squaring loop hits n-1 for an early witness (composite=false,
	// break) before a later witness rejects it.
	if millerRabinUint64(3277) {
		t.Error("millerRabinUint64(3277) should be composite")
	}
	// Small odd composite rejected on the very first witness with no continue.
	if millerRabinUint64(25) {
		t.Error("millerRabinUint64(25) should be composite")
	}
	// Prime whose witness set contains a value equal to n (a%n==0 → skip) yet is
	// still correctly reported prime: 7 is in the mrW4 base set {2,3,5,7}.
	if !millerRabinUint64(7) {
		t.Error("millerRabinUint64(7) should be prime")
	}
	// A genuine 30-bit prime (the isprime benchmark input, mrW4 band).
	if !millerRabinUint64(982451653) {
		t.Error("millerRabinUint64(982451653) should be prime")
	}
	// One prime from each larger magnitude band so mrWitnessesFor's tiers are all
	// exercised: mrW6 (~1e12), mrW9 (~1e15) and mrW12 (~1e19).
	for _, p := range []uint64{1000000000039, 1000000000000037, 18446744073709551557} {
		if !millerRabinUint64(p) {
			t.Errorf("millerRabinUint64(%d) should be prime", p)
		}
	}
}

// TestMillerRabinBase2Direct exercises the arbitrary-precision base-2 test's
// three exits directly: the immediate 2^d ≡ ±1 accept, the in-loop square that
// reaches n-1, and the composite reject.
func TestMillerRabinBase2Direct(t *testing.T) {
	if !millerRabinBase2(big.NewInt(7)) { // 2^3 ≡ 1 (mod 7): first branch
		t.Error("millerRabinBase2(7) should pass")
	}
	if !millerRabinBase2(big.NewInt(17)) { // a later square reaches 16: loop branch
		t.Error("millerRabinBase2(17) should pass")
	}
	if millerRabinBase2(big.NewInt(25)) { // never reaches n-1: false branch
		t.Error("millerRabinBase2(25) should fail")
	}
}

// TestIsPrimeBig drives IsPrime on values above 2^64, the only ones that take
// the arbitrary-precision path: a Mersenne prime, a small-factor composite
// (trial-division reject) and a large-factor composite (Baillie–PSW reject).
func TestIsPrimeBig(t *testing.T) {
	mers := bi(t, "170141183460469231731687303715884105727") // 2^127-1, prime
	if !IsPrime(mers) {
		t.Error("2^127-1 should be prime")
	}
	if IsPrime(new(big.Int).Mul(big.NewInt(3), mers)) { // small factor 3
		t.Error("3*(2^127-1) should be composite")
	}
	// (1e9+7)(1e9+9)(1000003) ≈ 1e24 > 2^64, odd, no factor <= 997.
	big3 := new(big.Int).Mul(new(big.Int).Mul(bi(t, "1000000007"), bi(t, "1000000009")), bi(t, "1000003"))
	if IsPrime(big3) {
		t.Error("1000000007*1000000009*1000003 should be composite")
	}
}

// TestLucasStrongLargePrimes exercises the strong Lucas test (and the Jacobi /
// Lucas-sequence helpers it drives) on large primes, whose longer binary
// expansions reach the doubling-and-step branches shorter inputs never do.
func TestLucasStrongLargePrimes(t *testing.T) {
	for _, s := range []string{"1000000007", "9223372036854775783", "18446744073709551557"} {
		if !lucasStrong(bi(t, s)) {
			t.Errorf("lucasStrong(%s) should be true", s)
		}
	}
}

// TestLucasStrongDirect drives the strong Lucas test on inputs that IsPrime's
// trial division would otherwise short-circuit, so the perfect-square,
// shared-factor (Jacobi 0) and composite-rejection branches are all covered.
func TestLucasStrongDirect(t *testing.T) {
	if lucasStrong(big.NewInt(25)) { // odd perfect square
		t.Error("lucasStrong(25) should be false (perfect square)")
	}
	if lucasStrong(big.NewInt(15)) { // D=5 shares a factor: Jacobi(5,15)=0
		t.Error("lucasStrong(15) should be false (shared factor)")
	}
	if lucasStrong(big.NewInt(2047)) { // base-2 pseudoprime, composite
		t.Error("lucasStrong(2047) should be false")
	}
	if !lucasStrong(big.NewInt(7919)) { // genuine prime
		t.Error("lucasStrong(7919) should be true")
	}
}
