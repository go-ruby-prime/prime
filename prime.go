// Copyright (c) the go-ruby-prime/prime authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package prime is a pure-Go (no cgo) reimplementation of Ruby's `prime`
// standard library — the deterministic, interpreter-independent core of MRI
// 4.0.5's Prime class and the Integer#prime? / Integer#prime_division refinements.
//
// It exposes the prime generator (Prime.each / Prime.take / Prime.first), the
// primality test (Prime.prime? / Integer#prime?), and the prime factorisation
// (Prime.prime_division / Integer#prime_division) together with its inverse
// (Prime.int_from_prime_division), all matching MRI byte-for-byte on the integer
// value model.
//
// Primality trial-divides by the small primes up to 37 for tiny inputs. Any
// value that fits in a uint64 is then settled by a deterministic Miller–Rabin
// test over the smallest witness set proven to have no composite counterexample
// for that magnitude ({2,7,61} below ~4.76e9, growing to the twelve-base set
// {2,3,5,7,…,37} for the whole 64-bit range). Each round runs in machine-word
// arithmetic with no heap allocation: below 2^32 a plain uint64 multiply suffices,
// and for the full range Montgomery multiplication replaces the per-step 64-bit
// division with a multiply, add and shift. Genuinely larger values fall through
// to a deterministic Baillie–PSW test (a strong base-2 Miller–Rabin combined with
// a strong Lucas test) evaluated with math/big. Both paths agree exactly on the
// 64-bit range (each is proven exact there) and BPSW remains a correct
// probable-prime test beyond it — exactly the guarantee MRI's own generator
// relies on.
package prime

import (
	"math/big"
	"math/bits"
	"sync"
)

// Pre-allocated big.Int constants reused throughout, to avoid re-parsing.
var (
	bigZero   = big.NewInt(0)
	bigOne    = big.NewInt(1)
	bigTwo    = big.NewInt(2)
	bigThree  = big.NewInt(3)
	bigNegOne = big.NewInt(-1)
)

// ZeroError is returned (as a panic value, mirroring MRI's ZeroDivisionError)
// when a factorisation is asked for zero. MRI raises ZeroDivisionError from
// Prime.prime_division(0); callers that prefer an error can use the
// PrimeDivisionErr variant.
type ZeroError struct{}

func (ZeroError) Error() string { return "ZeroDivisionError" }

// IsPrime reports whether n is prime, matching Ruby's Prime.prime?(n) /
// Integer#prime?. Numbers < 2 (including negatives and zero) are not prime;
// 2 and 3 are prime; every Carmichael number and Miller–Rabin/Lucas
// pseudoprime in the 64-bit range is correctly rejected.
//
// n is not mutated.
func IsPrime(n *big.Int) bool {
	if n == nil || n.Sign() < 1 {
		return false
	}
	// Everything that fits in a machine word is settled allocation-free by the
	// uint64 fast path (trial division then deterministic Miller–Rabin); only
	// genuinely larger values need the arbitrary-precision Baillie–PSW test.
	if n.IsUint64() {
		return isPrimeUint64(n.Uint64())
	}
	return isPrimeBig(n)
}

// isPrimeBig is the arbitrary-precision path, taken only for n >= 2^64 (so n is
// far above every small prime). It strips any small factor, then applies
// deterministic Baillie–PSW.
func isPrimeBig(n *big.Int) bool {
	if n.Bit(0) == 0 {
		return false // even and >= 2^64
	}
	// Trial-divide by the small primes to remove the small-factor cases the strong
	// tests must not be asked about.
	for _, p := range smallPrimes {
		if p == 2 {
			continue
		}
		if new(big.Int).Mod(n, big.NewInt(p)).Sign() == 0 {
			return false
		}
	}
	return millerRabinBase2(n) && lucasStrong(n)
}

// isPrimeUint64 reports whether the machine-word value n is prime, matching
// IsPrime exactly across the whole uint64 range. It trial-divides by the small
// primes, decides conclusively when n is within their squared reach, and
// otherwise runs deterministic Miller–Rabin — all in word arithmetic, no heap.
func isPrimeUint64(n uint64) bool {
	if n < 2 {
		return false // 0, 1
	}
	if n < 4 {
		return true // 2, 3
	}
	if n&1 == 0 {
		return false // even and > 2
	}
	for _, p := range mrPrefilter {
		if n == p {
			return true
		}
		if n%p == 0 {
			return false
		}
	}
	// No factor <= 37: any composite this small would have one, so n <= 37^2 is prime.
	if n <= mrPrefilterBound {
		return true
	}
	return millerRabinUint64(n)
}

// smallPrimes lists every prime <= 1000; it is the trial-division wheel that the
// factorisation (PrimeDivision) and the arbitrary-precision primality path share.
var smallPrimes = sievePrimes(1000)

// mrPrefilter is the small-prime sieve the uint64 primality test trial-divides by
// before any Miller–Rabin round: it rejects the overwhelming majority of
// composites for the cost of a handful of machine-word remainders, and — since
// every composite <= mrPrefilterBound has a factor among these primes — it settles
// primality outright below that bound. Keeping the set tiny (primes up to 37)
// avoids the ~160 redundant divisions the full wheel would otherwise spend on a
// large prime before Miller–Rabin, which dominate that input's cost.
var mrPrefilter = [...]uint64{3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37}

// mrPrefilterBound is 37*37: any composite at or below it has a prime factor <= 37,
// so surviving mrPrefilter with n <= mrPrefilterBound proves n prime.
const mrPrefilterBound = 37 * 37

// Miller–Rabin witness sets, smallest first. Each is proven to have no composite
// strong-pseudoprime below the paired bound, so a value is tested with the fewest
// bases its magnitude allows — fewer bases means fewer modular exponentiations
// while staying exact. Bounds: Jaeschke (1993) for the sets through nine bases;
// Sorenson & Webster (2015) for the twelve-base set, whose bound (~3.3e23) covers
// the entire uint64 range.
var (
	mrW3  = [...]uint64{2, 7, 61}                       // n < 4_759_123_141
	mrW5  = [...]uint64{2, 3, 5, 7, 11}                 // n < 2_152_302_898_747
	mrW7  = [...]uint64{2, 3, 5, 7, 11, 13, 17}         // n < 341_550_071_728_321
	mrW9  = [...]uint64{2, 3, 5, 7, 11, 13, 17, 19, 23} // n < 3_825_123_056_546_413_051
	mrW12 = [...]uint64{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37}
)

// mrWitnessesFor returns the smallest proven-deterministic base set for n.
func mrWitnessesFor(n uint64) []uint64 {
	switch {
	case n < 4759123141:
		return mrW3[:]
	case n < 2152302898747:
		return mrW5[:]
	case n < 341550071728321:
		return mrW7[:]
	case n < 3825123056546413051:
		return mrW9[:]
	default:
		return mrW12[:]
	}
}

// millerRabinUint64 is the deterministic Miller–Rabin test for an odd n > 3; it is
// exact for every such n < 2^64 (see mrWitnessesFor). It keeps two allocation-free,
// division-light multiply strategies: below 2^32 the product of two residues fits
// in a uint64, so a single machine multiply-and-remainder suffices; for the full
// range it uses Montgomery multiplication, which trades the per-step 64-bit
// division for a multiply, an add and a shift.
func millerRabinUint64(n uint64) bool {
	// Write n-1 = d * 2^s with d odd.
	s := bits.TrailingZeros64(n - 1)
	d := (n - 1) >> s
	witnesses := mrWitnessesFor(n)
	if n < 1<<32 {
		for _, a := range witnesses {
			if a >= n {
				// Only reachable when millerRabinUint64 is invoked directly on a tiny
				// n (IsPrime never routes n <= 37^2 here): reduce the base mod n and
				// skip it when it vanishes, exactly as classical Miller–Rabin does.
				if a %= n; a == 0 {
					continue
				}
			}
			if !mrProbeSmall(a, d, n, s) {
				return false
			}
		}
		return true
	}
	// n >= 2^32 is odd and exceeds every witness (<= 37), so no base needs
	// reduction. The Montgomery constants are computed once for the modulus.
	np, one := montSetup(n)
	nm1 := n - one // Montgomery form of n-1 (= -R mod n)
	for _, a := range witnesses {
		if !mrProbeMont(a, d, n, s, np, one, nm1) {
			return false
		}
	}
	return true
}

// mrProbeSmall runs one Miller–Rabin round for base a on n < 2^32, where every
// residue product fits in a uint64. It reports whether a witnesses n's primality
// (true = probable prime for this base; false = n is definitely composite).
func mrProbeSmall(a, d, n uint64, s int) bool {
	x := powmodSmall(a, d, n)
	if x == 1 || x == n-1 {
		return true
	}
	for i := 1; i < s; i++ {
		x = x * x % n // x < n < 2^32, so x*x < 2^64
		if x == n-1 {
			return true
		}
	}
	return false
}

// powmodSmall returns a^e mod n for n < 2^32 by square-and-multiply with plain
// machine-word multiplies (no 128-bit intermediate is needed).
func powmodSmall(a, e, n uint64) uint64 {
	r := uint64(1)
	a %= n
	for e > 0 {
		if e&1 == 1 {
			r = r * a % n
		}
		a = a * a % n
		e >>= 1
	}
	return r
}

// mrProbeMont runs one Miller–Rabin round for base a using Montgomery arithmetic
// (n >= 2^32). np and one are the Montgomery constants for n; nm1 is the
// Montgomery representative of n-1.
func mrProbeMont(a, d, n uint64, s int, np, one, nm1 uint64) bool {
	x := powmodMont(toMont(a, n), d, n, np, one)
	if x == one || x == nm1 {
		return true
	}
	for i := 1; i < s; i++ {
		x = montMul(x, x, n, np)
		if x == nm1 {
			return true
		}
	}
	return false
}

// --- Montgomery modular arithmetic (odd modulus n, R = 2^64) ----------------

// montSetup returns n' = -n^{-1} mod 2^64 and one = R mod n (the Montgomery form
// of 1). n must be odd. n' is found by Newton's iteration, which doubles the
// number of correct low bits each step: six steps take the 1-bit seed (correct
// mod 2 because n is odd) to the full 64 bits.
func montSetup(n uint64) (nprime, one uint64) {
	inv := uint64(1)
	for i := 0; i < 6; i++ {
		inv *= 2 - n*inv
	}
	// n*inv ≡ 1 (mod 2^64) ⇒ n*(-inv) ≡ -1; and (2^64 - n) mod n == 2^64 mod n.
	return -inv, (-n) % n
}

// toMont converts a (with a < n) to Montgomery form a*R mod n. a*2^64 mod n is
// exactly the remainder of the 128-bit value (a, 0) divided by n, and a < n keeps
// that quotient inside a uint64.
func toMont(a, n uint64) uint64 {
	_, r := bits.Div64(a, 0, n)
	return r
}

// montMul returns the Montgomery product of a and b (both < n): a*b*R^{-1} mod n.
func montMul(a, b, n, nprime uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	// REDC: choose m so the low 64 bits of T + m*n vanish, then shift down by R.
	m := lo * nprime
	mnHi, mnLo := bits.Mul64(m, n)
	_, carry := bits.Add64(lo, mnLo, 0)
	res, carry2 := bits.Add64(hi, mnHi, carry)
	if carry2 != 0 || res >= n {
		res -= n
	}
	return res
}

// powmodMont returns base^e in Montgomery form, where base is already in
// Montgomery form and one is the Montgomery form of 1.
func powmodMont(base, e, n, nprime, one uint64) uint64 {
	r := one
	for e > 0 {
		if e&1 == 1 {
			r = montMul(r, base, n, nprime)
		}
		base = montMul(base, base, n, nprime)
		e >>= 1
	}
	return r
}

// sievePrimes returns every prime <= limit via a simple sieve of Eratosthenes.
func sievePrimes(limit int64) []int64 {
	if limit < 2 {
		return nil
	}
	composite := make([]bool, limit+1)
	var out []int64
	for i := int64(2); i <= limit; i++ {
		if composite[i] {
			continue
		}
		out = append(out, i)
		for j := i * i; j <= limit; j += i {
			composite[j] = true
		}
	}
	return out
}

// millerRabinBase2 is a strong probable-prime test to base 2. n must be odd and
// greater than 3. It is the first half of Baillie–PSW.
func millerRabinBase2(n *big.Int) bool {
	// Write n-1 = d * 2^s with d odd.
	nm1 := new(big.Int).Sub(n, bigOne)
	d := new(big.Int).Set(nm1)
	s := 0
	for d.Bit(0) == 0 {
		d.Rsh(d, 1)
		s++
	}
	// x = 2^d mod n.
	x := new(big.Int).Exp(bigTwo, d, n)
	if x.Cmp(bigOne) == 0 || x.Cmp(nm1) == 0 {
		return true
	}
	for i := 0; i < s-1; i++ {
		x.Mul(x, x)
		x.Mod(x, n)
		if x.Cmp(nm1) == 0 {
			return true
		}
	}
	return false
}

// lucasStrong is the strong Lucas probable-prime test with Selfridge's
// parameters (the second half of Baillie–PSW). n must be odd, > 3 and not a
// perfect square.
func lucasStrong(n *big.Int) bool {
	if isPerfectSquare(n) {
		return false
	}
	// Selfridge: find the first D in 5, -7, 9, -11, ... with Jacobi(D, n) == -1.
	D := big.NewInt(5)
	for {
		j := jacobi(D, n)
		if j == -1 {
			break
		}
		if j == 0 {
			// gcd(D, n) shares a factor with n => composite (n has no small factor
			// here, so |D| must equal n, which only happens for tiny n already
			// handled). Treat as composite.
			return false
		}
		// Next D: negate and step magnitude by 2.
		if D.Sign() > 0 {
			D.Add(D, bigTwo)
		} else {
			D.Sub(D, bigTwo)
		}
		D.Neg(D)
	}
	P := bigOne
	Q := new(big.Int).Sub(bigOne, D) // Q = (1 - D) / 4
	Q.Div(Q, big.NewInt(4))

	// Compute the strong Lucas test. d = n+1 = d2 * 2^r with d2 odd.
	d := new(big.Int).Add(n, bigOne)
	r := 0
	for d.Bit(0) == 0 {
		d.Rsh(d, 1)
		r++
	}

	U, V, Qk := lucasUV(d, P, Q, n)
	if U.Sign() == 0 || V.Sign() == 0 {
		return true
	}
	for i := 0; i < r-1; i++ {
		// V_{2k} = V_k^2 - 2*Q^k
		V.Mul(V, V)
		t := new(big.Int).Lsh(Qk, 1)
		V.Sub(V, t)
		V.Mod(V, n)
		if V.Sign() == 0 {
			return true
		}
		Qk.Mul(Qk, Qk)
		Qk.Mod(Qk, n)
	}
	return false
}

// lucasUV computes U_k, V_k (mod n) and Q^k (mod n) for the Lucas sequence with
// parameters P, Q, using the binary expansion of k.
func lucasUV(k *big.Int, P, Q, n *big.Int) (Uk, Vk, Qk *big.Int) {
	U := big.NewInt(1)       // U_1
	V := new(big.Int).Set(P) // V_1
	Qk = new(big.Int).Set(Q) // Q^1

	inv2 := new(big.Int).ModInverse(bigTwo, n)

	for i := k.BitLen() - 2; i >= 0; i-- {
		// Doubling: U_{2j} = U_j V_j; V_{2j} = V_j^2 - 2 Q^j.
		U.Mul(U, V)
		U.Mod(U, n)

		t := new(big.Int).Lsh(Qk, 1)
		V.Mul(V, V)
		V.Sub(V, t)
		V.Mod(V, n)

		Qk.Mul(Qk, Qk)
		Qk.Mod(Qk, n)

		if k.Bit(i) == 1 {
			// Increment index by one.
			// U' = (P U + V) / 2 ; V' = (D U + P V) / 2  (mod n)
			PU := new(big.Int).Mul(P, U)
			newU := new(big.Int).Add(PU, V)
			newU.Mul(newU, inv2)
			newU.Mod(newU, n)

			DU := new(big.Int).Mul(new(big.Int).Sub(new(big.Int).Mul(P, P), new(big.Int).Lsh(Q, 2)), U)
			PV := new(big.Int).Mul(P, V)
			newV := new(big.Int).Add(DU, PV)
			newV.Mul(newV, inv2)
			newV.Mod(newV, n)

			U = newU
			V = newV

			Qk.Mul(Qk, Q)
			Qk.Mod(Qk, n)
		}
	}
	U.Mod(U, n)
	V.Mod(V, n)
	return U, V, Qk
}

// jacobi computes the Jacobi symbol (a/n) for odd n > 0.
func jacobi(a, n *big.Int) int {
	a = new(big.Int).Mod(a, n)
	nn := new(big.Int).Set(n)
	result := 1
	for a.Sign() != 0 {
		for a.Bit(0) == 0 {
			a.Rsh(a, 1)
			r := new(big.Int).Mod(nn, big.NewInt(8)).Int64()
			if r == 3 || r == 5 {
				result = -result
			}
		}
		a, nn = nn, a
		if new(big.Int).Mod(a, big.NewInt(4)).Int64() == 3 &&
			new(big.Int).Mod(nn, big.NewInt(4)).Int64() == 3 {
			result = -result
		}
		a.Mod(a, nn)
	}
	if nn.Cmp(bigOne) == 0 {
		return result
	}
	return 0
}

// isPerfectSquare reports whether n is a perfect square.
func isPerfectSquare(n *big.Int) bool {
	if n.Sign() < 0 {
		return false
	}
	r := new(big.Int).Sqrt(n)
	r.Mul(r, r)
	return r.Cmp(n) == 0
}

// PrimeDivision returns the prime factorisation of n as Ruby's
// Prime.prime_division(n) / Integer#prime_division does: a slice of [prime,
// exponent] pairs in ascending prime order. For negative n a leading [-1, 1]
// pair carries the sign (matching MRI). It panics with ZeroError for n == 0,
// mirroring MRI's ZeroDivisionError; PrimeDivisionErr is the non-panicking form.
//
// n is not mutated.
func PrimeDivision(n *big.Int) [][2]*big.Int {
	pairs, err := PrimeDivisionErr(n)
	if err != nil {
		panic(err)
	}
	return pairs
}

// PrimeDivisionErr is PrimeDivision without the panic: it returns a ZeroError for
// n == 0 instead of panicking. The factorisation is otherwise identical.
func PrimeDivisionErr(n *big.Int) ([][2]*big.Int, error) {
	if n == nil || n.Sign() == 0 {
		return nil, ZeroError{}
	}
	m := new(big.Int).Set(n)
	var pairs [][2]*big.Int
	if m.Sign() < 0 {
		pairs = append(pairs, [2]*big.Int{new(big.Int).Set(bigNegOne), big.NewInt(1)})
		m.Neg(m)
	}
	// Factor out each small prime, then the remaining cofactor.
	for _, p := range smallPrimes {
		pp := big.NewInt(p)
		if new(big.Int).Mul(pp, pp).Cmp(m) > 0 {
			break
		}
		if new(big.Int).Mod(m, pp).Sign() == 0 {
			exp := int64(0)
			for new(big.Int).Mod(m, pp).Sign() == 0 {
				m.Div(m, pp)
				exp++
			}
			pairs = append(pairs, [2]*big.Int{pp, big.NewInt(exp)})
		}
	}
	if m.Cmp(bigOne) > 0 {
		// Whatever remains is prime (cofactor with no small factor and a square root
		// below the small-prime ceiling) or a large prime/semiprime; factor it with
		// the general routine.
		for _, pe := range factorLarge(m) {
			pairs = append(pairs, pe)
		}
	}
	return pairs, nil
}

// factorLarge factorises a value with no small prime factor, returning ascending
// [prime, exponent] pairs. It uses Pollard's rho (Brent's variant) recursively,
// terminating each branch at a proven prime via IsPrime.
func factorLarge(n *big.Int) [][2]*big.Int {
	counts := map[string]int64{}
	primes := map[string]*big.Int{}
	// recurse factors m (always >= 2: the caller passes the >1 cofactor and
	// pollardRho only ever returns a proper divisor 1 < d < m, so m/d >= 2 too).
	var recurse func(*big.Int)
	recurse = func(m *big.Int) {
		if IsPrime(m) {
			k := m.String()
			counts[k]++
			primes[k] = new(big.Int).Set(m)
			return
		}
		d := pollardRho(m)
		recurse(d)
		recurse(new(big.Int).Div(m, d))
	}
	recurse(n)

	// Sort the discovered primes ascending.
	keys := make([]*big.Int, 0, len(primes))
	for _, p := range primes {
		keys = append(keys, p)
	}
	sortBigInts(keys)
	out := make([][2]*big.Int, 0, len(keys))
	for _, p := range keys {
		out = append(out, [2]*big.Int{p, big.NewInt(counts[p.String()])})
	}
	return out
}

// pollardRho returns a non-trivial factor of composite n (Brent's improvement).
func pollardRho(n *big.Int) *big.Int {
	if n.Bit(0) == 0 {
		return big.NewInt(2)
	}
	x := big.NewInt(2)
	y := big.NewInt(2)
	c := big.NewInt(1)
	d := big.NewInt(1)
	f := func(v *big.Int) *big.Int {
		r := new(big.Int).Mul(v, v)
		r.Add(r, c)
		r.Mod(r, n)
		return r
	}
	for {
		for d.Cmp(bigOne) == 0 {
			x = f(x)
			y = f(f(y))
			diff := new(big.Int).Sub(x, y)
			diff.Abs(diff)
			if diff.Sign() == 0 {
				break
			}
			d.GCD(nil, nil, diff, n)
		}
		if d.Cmp(n) != 0 && d.Cmp(bigOne) != 0 {
			return d
		}
		// Retry with a different polynomial constant.
		c.Add(c, bigOne)
		x.SetInt64(2)
		y.SetInt64(2)
		d.SetInt64(1)
	}
}

// sortBigInts sorts s ascending in place (insertion sort; the factor count is
// tiny — bounded by the number of distinct primes).
func sortBigInts(s []*big.Int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Cmp(s[j-1]) < 0; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Int reconstructs an integer from a prime-division slice, matching Ruby's
// Prime.int_from_prime_division: it multiplies prime**exponent over the pairs
// (a leading [-1, 1] applies the sign). Exponents are taken as non-negative
// integers; the result is exact for the integer case MRI's Integer model uses.
func Int(pairs [][2]*big.Int) *big.Int {
	result := big.NewInt(1)
	for _, pe := range pairs {
		p, e := pe[0], pe[1]
		term := new(big.Int).Exp(p, e, nil)
		result.Mul(result, term)
	}
	return result
}

// The sequential prime generator (Each / Take / First / EachPrime) is backed by
// a process-wide memoized, incrementally-grown sieve — the analogue of MRI's
// Prime singleton, whose EratosthenesGenerator caches an ever-growing table of
// discovered primes so repeated enumerations never redo work. Without this cache
// each enumeration re-ran a full Baillie–PSW primality test (with big.Int
// trial division allocating hundreds of temporaries) per candidate, which made
// small enumerations an order of magnitude slower than the reference. The sieve
// keeps int64 primes and only widens; primeMu guards it.
var (
	primeMu sync.Mutex
	// cachedPrimes holds every prime <= sievedTo, in ascending order. It only
	// grows (append-only, never reordered), so a snapshot of the slice header
	// taken under the lock stays valid to read after the lock is released.
	cachedPrimes = []int64{2}
	sievedTo     = int64(2)
)

// growPrimesUpToLocked extends cachedPrimes to include every prime <= limit.
// primeMu must be held. It sieves in segments whose top never exceeds the
// square of the currently-known ceiling, so cachedPrimes always already holds
// the base primes (those <= sqrt(top)) each segment needs.
func growPrimesUpToLocked(limit int64) {
	for sievedTo < limit {
		newTop := limit
		if maxByBase := sievedTo * sievedTo; newTop > maxByBase {
			newTop = maxByBase
		}
		sieveSegmentLocked(sievedTo+1, newTop)
		sievedTo = newTop
	}
}

// growPrimesCountLocked extends cachedPrimes until it holds at least n primes.
// primeMu must be held.
func growPrimesCountLocked(n int) {
	for len(cachedPrimes) < n {
		top := sievedTo * 2
		if top < 16 {
			top = 16
		}
		growPrimesUpToLocked(top)
	}
}

// sieveSegmentLocked sieves the closed range [lo, hi] using the already-known
// base primes and appends every prime it finds to cachedPrimes in ascending
// order. primeMu must be held; callers guarantee 2 <= lo <= hi and that
// cachedPrimes covers all primes <= sqrt(hi).
func sieveSegmentLocked(lo, hi int64) {
	seg := make([]bool, hi-lo+1) // false = still prime
	for _, p := range cachedPrimes {
		if p*p > hi {
			break
		}
		start := p * p
		if start < lo {
			start = ((lo + p - 1) / p) * p
		}
		for j := start; j <= hi; j += p {
			seg[j-lo] = true
		}
	}
	for i := int64(0); i < int64(len(seg)); i++ {
		if !seg[i] {
			cachedPrimes = append(cachedPrimes, lo+i)
		}
	}
}

// primeAt returns the idx-th prime (0 -> 2, 1 -> 3, ...), growing the cache as
// needed. It locks primeMu.
func primeAt(idx int) int64 {
	primeMu.Lock()
	defer primeMu.Unlock()
	growPrimesCountLocked(idx + 1)
	return cachedPrimes[idx]
}

// Each yields every prime p with p <= ubound, in ascending order, calling yield
// for each. If yield returns false, iteration stops early. This is the bounded
// form of Ruby's Prime.each(ubound) { |p| ... }; the unbounded generator is
// served by Take / First / EachPrime, which never need an upper bound.
//
// A non-positive ubound yields nothing.
func Each(ubound int64, yield func(p *big.Int) bool) {
	if ubound < 2 {
		return
	}
	primeMu.Lock()
	growPrimesUpToLocked(ubound)
	snap := cachedPrimes // append-only backing array: safe to read unlocked
	primeMu.Unlock()
	for _, p := range snap {
		if p > ubound {
			return
		}
		if !yield(big.NewInt(p)) {
			return
		}
	}
}

// Take returns the first n primes (2, 3, 5, ...), matching Ruby's
// Prime.take(n) / Prime.first(n). A non-positive n returns an empty slice.
func Take(n int) []*big.Int {
	if n <= 0 {
		return []*big.Int{}
	}
	primeMu.Lock()
	growPrimesCountLocked(n)
	out := make([]*big.Int, n)
	for i := 0; i < n; i++ {
		out[i] = big.NewInt(cachedPrimes[i])
	}
	primeMu.Unlock()
	return out
}

// First is an alias for Take, mirroring Prime.first(n) == Prime.take(n).
func First(n int) []*big.Int { return Take(n) }

// EachPrime returns a stateful generator: each call returns the next prime,
// starting at 2 and continuing forever (it is the unbounded Prime.each
// enumerator). It is the building block behind the Prev / Next cursor and reads
// from the shared memoized sieve, so successive calls are amortised O(1).
func EachPrime() func() *big.Int {
	idx := 0
	return func() *big.Int {
		p := primeAt(idx)
		idx++
		return big.NewInt(p)
	}
}

// nextPrime returns the smallest prime strictly greater than n.
func nextPrime(n *big.Int) *big.Int {
	c := new(big.Int).Set(n)
	if c.Cmp(bigTwo) < 0 {
		return big.NewInt(2)
	}
	if c.Cmp(bigTwo) == 0 {
		return big.NewInt(3)
	}
	// Step to the next odd candidate and onward by 2.
	if c.Bit(0) == 0 {
		c.Add(c, bigOne)
	} else {
		c.Add(c, bigTwo)
	}
	for !IsPrime(c) {
		c.Add(c, bigTwo)
	}
	return c
}

// prevPrime returns the largest prime strictly less than n, or nil when none
// exists (n <= 2).
func prevPrime(n *big.Int) *big.Int {
	c := new(big.Int).Set(n)
	if c.Cmp(bigThree) <= 0 {
		if c.Cmp(bigThree) == 0 {
			return big.NewInt(2)
		}
		return nil
	}
	if c.Bit(0) == 0 {
		c.Sub(c, bigOne)
	} else {
		c.Sub(c, bigTwo)
	}
	// c is now an odd value >= 3; stepping down by 2 always reaches 3 (prime), so
	// the loop is guaranteed to return.
	for {
		if IsPrime(c) {
			return c
		}
		c.Sub(c, bigTwo)
	}
}

// Next returns the smallest prime strictly greater than n. n is not mutated.
func Next(n *big.Int) *big.Int { return nextPrime(n) }

// Prev returns the largest prime strictly less than n, or nil when none exists
// (n <= 2). n is not mutated.
func Prev(n *big.Int) *big.Int { return prevPrime(n) }
