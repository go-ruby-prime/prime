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
// Primality uses a small wheel-assisted trial division for tiny inputs and a
// deterministic Baillie–PSW test (a strong base-2 Miller–Rabin combined with a
// strong Lucas test) for everything else. BPSW has no known counterexample and
// is proven to have none below 2^64, so the result is exact across the entire
// 64-bit range and a correct probable-prime test beyond it — exactly the
// guarantee MRI's own generator relies on.
package prime

import (
	"math/big"
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
	if n.Cmp(bigThree) <= 0 {
		// 1 -> false, 2 -> true, 3 -> true.
		return n.Cmp(bigOne) > 0
	}
	if n.Bit(0) == 0 {
		return false // even and > 2
	}
	// Trial-divide by the small primes first; this both fast-paths small inputs
	// and removes the small-factor cases the strong tests must not be asked about.
	for _, p := range smallPrimes {
		if p == 2 {
			continue
		}
		pp := big.NewInt(p)
		if n.Cmp(pp) == 0 {
			return true
		}
		m := new(big.Int).Mod(n, pp)
		if m.Sign() == 0 {
			return false
		}
	}
	// For values within trial-division reach of the small-prime table there can be
	// no remaining composite (largest small prime squared bounds it); otherwise run
	// Baillie–PSW.
	if n.Cmp(smallPrimeBound) <= 0 {
		return true
	}
	return millerRabinBase2(n) && lucasStrong(n)
}

// smallPrimes is the trial-division wheel: primes up to 1000. smallPrimeBound is
// the square of the largest, below which trial division alone is conclusive.
var (
	smallPrimes     = sievePrimes(1000)
	smallPrimeBound = func() *big.Int {
		last := smallPrimes[len(smallPrimes)-1]
		b := big.NewInt(last)
		return b.Mul(b, b)
	}()
)

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
	for _, p := range sievePrimes(ubound) {
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
	out := make([]*big.Int, 0, n)
	it := EachPrime()
	for i := 0; i < n; i++ {
		out = append(out, it())
	}
	return out
}

// First is an alias for Take, mirroring Prime.first(n) == Prime.take(n).
func First(n int) []*big.Int { return Take(n) }

// EachPrime returns a stateful generator: each call returns the next prime,
// starting at 2 and continuing forever (it is the unbounded Prime.each
// enumerator). It is the building block behind Take / First and the Prev / Next
// cursor.
func EachPrime() func() *big.Int {
	cur := big.NewInt(1)
	return func() *big.Int {
		cur = nextPrime(cur)
		return new(big.Int).Set(cur)
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
