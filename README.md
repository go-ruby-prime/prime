<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-prime/brand/main/social/go-ruby-prime-prime.png" alt="go-ruby-prime/prime" width="720"></p>

# prime — go-ruby-prime

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-prime.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Ruby's
[`prime`](https://docs.ruby-lang.org/en/master/Prime.html) standard library** —
the deterministic, interpreter-independent core of MRI 4.0.5's `Prime` class and
the `Integer#prime?` / `Integer#prime_division` refinements. It generates the
primes, tests primality, factorises an integer and reconstructs it — matching
MRI byte-for-byte on the integer value model, **without any Ruby runtime**.

It is the `prime` backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime — a sibling
of [go-ruby-regexp](https://github.com/go-ruby-regexp/regexp) (the Onigmo engine),
[go-ruby-yaml](https://github.com/go-ruby-yaml/yaml) (the Psych port) and
[go-ruby-marshal](https://github.com/go-ruby-marshal/marshal).

> **What it is — and isn't.** The number theory behind `Prime` — sieving the
> primes, testing primality, factorising an integer — is fully deterministic and
> needs **no interpreter**, so it lives here as pure Go. Binding it to Ruby
> objects (the `Prime.each` enumerator, `Integer#prime?`) is the host's job; this
> library hands back `*big.Int` values the host wraps in its own `Integer`.

## Features

Faithful port of `prime`, validated against the `ruby` binary on every supported
platform:

- **The generator** — `Take(n)` / `First(n)` return the first *n* primes
  (`Prime.take` / `Prime.first`); `Each(ubound, yield)` enumerates every prime
  `p <= ubound` (`Prime.each(ubound)`); `EachPrime()` is the unbounded cursor.
- **Primality** — `IsPrime(n)` mirrors `Prime.prime?` / `Integer#prime?` exactly:
  numbers `< 2` are not prime, and **every Carmichael number** (561, 1105, …) and
  **strong pseudoprime** (2047, 3215031751, …) is correctly rejected. Small inputs
  use trial division; everything else uses a **deterministic Baillie–PSW** test
  (strong base-2 Miller–Rabin + strong Lucas), exact across the whole 64-bit range.
- **Factorisation** — `PrimeDivision(n)` returns `[[p, exp], …]` in ascending
  prime order (`Prime.prime_division` / `Integer#prime_division`), with a leading
  `[-1, 1]` for negative *n* and a `ZeroError` panic (MRI's `ZeroDivisionError`)
  for 0. Large cofactors fall back to **Pollard's rho**.
- **Reconstruction** — `Int(pairs)` multiplies `prime**exp` back to the integer
  (`Prime.int_from_prime_division`), the inverse of `PrimeDivision`.
- **Cursor** — `Next(n)` / `Prev(n)` step to the adjacent prime.

CGO-free, dependency-free (only `math/big`), **100% test coverage**, `gofmt` +
`go vet` clean, and green across the six 64-bit Go targets (amd64, arm64,
riscv64, loong64, ppc64le, s390x).

## Install

```sh
go get github.com/go-ruby-prime/prime
```

## Usage

```go
package main

import (
	"fmt"
	"math/big"

	"github.com/go-ruby-prime/prime"
)

func main() {
	fmt.Println(prime.Take(5))                       // [2 3 5 7 11]   (Prime.take 5)
	fmt.Println(prime.IsPrime(big.NewInt(561)))      // false          (Carmichael)
	fmt.Println(prime.IsPrime(big.NewInt(7919)))     // true
	fmt.Println(prime.PrimeDivision(big.NewInt(12))) // [[2 2] [3 1]]  (prime_division)
	fmt.Println(prime.PrimeDivision(big.NewInt(-12)))// [[-1 1] [2 2] [3 1]]

	// Reconstruct the integer from its factorisation.
	n := prime.Int(prime.PrimeDivision(big.NewInt(360)))
	fmt.Println(n) // 360

	// Bounded enumeration (Prime.each(11)).
	prime.Each(11, func(p *big.Int) bool { fmt.Print(p, " "); return true })
	fmt.Println() // 2 3 5 7 11
}
```

## API

```go
// IsPrime reports whether n is prime (Prime.prime? / Integer#prime?).
func IsPrime(n *big.Int) bool

// Take / First return the first n primes (Prime.take / Prime.first).
func Take(n int) []*big.Int
func First(n int) []*big.Int

// Each yields every prime p <= ubound, stopping early if yield returns false
// (Prime.each(ubound) { |p| ... }).
func Each(ubound int64, yield func(p *big.Int) bool)

// EachPrime returns a stateful generator: each call returns the next prime,
// starting at 2 and continuing forever (the unbounded Prime.each enumerator).
func EachPrime() func() *big.Int

// PrimeDivision returns the [prime, exponent] pairs of n in ascending order
// (Prime.prime_division / Integer#prime_division); a leading [-1, 1] carries a
// negative sign. It panics with ZeroError for n == 0 (MRI's ZeroDivisionError);
// PrimeDivisionErr is the non-panicking form.
func PrimeDivision(n *big.Int) [][2]*big.Int
func PrimeDivisionErr(n *big.Int) ([][2]*big.Int, error)

// Int reconstructs the integer from a prime-division slice
// (Prime.int_from_prime_division), the inverse of PrimeDivision.
func Int(pairs [][2]*big.Int) *big.Int

// Next / Prev return the adjacent prime (Prev returns nil when none exists).
func Next(n *big.Int) *big.Int
func Prev(n *big.Int) *big.Int

type ZeroError struct{} // mirrors Ruby's ZeroDivisionError
```

## Ruby ↔ Go value model

Every integer flows through `*big.Int`, so a host can map its own `Integer` to and
from this package without precision loss.

| Ruby                                 | Go                            |
| ------------------------------------ | ----------------------------- |
| `Prime.prime?(n)` / `n.prime?`       | `IsPrime(n)`                  |
| `Prime.take(n)` / `Prime.first(n)`   | `Take(n)` / `First(n)`        |
| `Prime.each(ubound) { ... }`         | `Each(ubound, yield)`         |
| `Prime.each` (enumerator)            | `EachPrime()`                 |
| `Prime.prime_division(n)` / `n.prime_division` | `PrimeDivision(n)`  |
| `Prime.int_from_prime_division(ps)`  | `Int(ps)`                     |
| `ZeroDivisionError`                  | `ZeroError` (panic)           |

## Algorithm

Primality is exact, not probabilistic, over the range Ruby programs use:

- **Small inputs** (`< 1000²`) are settled by **trial division** against the
  primes up to 1000.
- **Larger inputs** use **Baillie–PSW** — a strong base-2 Miller–Rabin test
  combined with a strong Lucas test (Selfridge parameters). BPSW has **no known
  counterexample** and is proven to have none below 2⁶⁴, so the result is exact
  across the entire 64-bit range and a sound probable-prime test beyond it.
- **Factorisation** strips small primes, then splits the cofactor with **Pollard's
  rho** (Brent's variant), recursing to proven primes.

## Tests & coverage

The suite pairs deterministic, ruby-free **golden tables** (which alone hold
coverage at 100%, so the qemu cross-arch and Windows lanes pass the gate) with a
**differential MRI oracle**: a corpus is computed here and by the system `ruby`
(`Prime.prime?`, `Prime.take`, `Prime.prime_division`, …) and the two are
compared. The oracle scripts `$stdout.binmode` / `$stdin.binmode` so Windows
text-mode never pollutes the bytes, gate themselves on `RUBY_VERSION >= "4.0"`,
and skip where `ruby` is absent.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-prime/prime authors.
