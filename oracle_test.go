// Copyright (c) the go-ruby-prime/prime authors
//
// SPDX-License-Identifier: BSD-3-Clause

package prime

import (
	"math/big"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// rubyBin locates a usable `ruby` once. The oracle tests skip themselves when it
// is absent (the qemu cross-arch lanes and the Windows lane), so the
// deterministic suite alone drives the 100% gate there. It also skips when the
// interpreter predates MRI 4.0, the reference this port tracks.
func rubyBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not on PATH; skipping MRI oracle")
	}
	// Gate the oracle on RUBY_VERSION >= "4.0" (the version this port mirrors).
	out, err := exec.Command(path, "-e", "print RUBY_VERSION").Output()
	if err != nil {
		t.Skipf("cannot query ruby version: %v", err)
	}
	if !rubyAtLeast4(string(out)) {
		t.Skipf("ruby %s < 4.0; skipping MRI oracle", strings.TrimSpace(string(out)))
	}
	return path
}

// rubyAtLeast4 reports whether the dotted RUBY_VERSION string is >= "4.0".
func rubyAtLeast4(v string) bool {
	v = strings.TrimSpace(v)
	major := v
	if i := strings.IndexByte(v, '.'); i >= 0 {
		major = v[:i]
	}
	n, err := strconv.Atoi(major)
	return err == nil && n >= 4
}

// rubyEval runs a `ruby -rprime` script and returns its stdout. The script binds
// $stdout.binmode / $stdin.binmode so Windows text-mode never pollutes the bytes
// (the go-ruby-erb lesson); the no-Windows CI lanes run it, the Windows lane
// skips via rubyBin.
func rubyEval(t *testing.T, bin, script string) string {
	t.Helper()
	pre := "$stdout.binmode\n$stdin.binmode\n"
	cmd := exec.Command(bin, "-rprime", "-e", pre+script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ruby error: %v\nscript:\n%s\noutput:\n%s", err, script, out)
	}
	return string(out)
}

// TestOraclePrimeP cross-checks IsPrime against MRI Prime.prime? over a corpus
// that spans small numbers, negatives, Carmichael numbers, base-2 pseudoprimes
// and a handful of moderate values MRI can still answer quickly.
func TestOraclePrimeP(t *testing.T) {
	bin := rubyBin(t)
	nums := []int64{
		-7, -1, 0, 1, 2, 3, 4, 5, 17, 25, 100, 561, 1105, 1729,
		2047, 3215031751, 7919, 104729, 1000003,
	}
	var b strings.Builder
	for _, n := range nums {
		b.WriteString("print(Prime.prime?(")
		b.WriteString(strconv.FormatInt(n, 10))
		b.WriteString(") ? \"1\" : \"0\")\n")
	}
	want := rubyEval(t, bin, b.String())
	var got strings.Builder
	for _, n := range nums {
		if IsPrime(big.NewInt(n)) {
			got.WriteByte('1')
		} else {
			got.WriteByte('0')
		}
	}
	if got.String() != want {
		t.Errorf("Prime.prime? mismatch\n go: %s\nmri: %s", got.String(), want)
	}
}

// TestOracleTakeFirst cross-checks Take / First against Prime.take / Prime.first.
func TestOracleTakeFirst(t *testing.T) {
	bin := rubyBin(t)
	for _, n := range []int{0, 1, 5, 20, 50} {
		want := strings.TrimSpace(rubyEval(t, bin,
			"print Prime.take("+strconv.Itoa(n)+").join(\",\")"))
		got := joinInts(Take(n))
		if got != want {
			t.Errorf("Take(%d)\n go: %q\nmri: %q", n, got, want)
		}
		want = strings.TrimSpace(rubyEval(t, bin,
			"print Prime.first("+strconv.Itoa(n)+").join(\",\")"))
		if got2 := joinInts(First(n)); got2 != want {
			t.Errorf("First(%d)\n go: %q\nmri: %q", n, got2, want)
		}
	}
}

// TestOracleEach cross-checks the bounded Each against Prime.each(ubound).
func TestOracleEach(t *testing.T) {
	bin := rubyBin(t)
	for _, ub := range []int64{0, 1, 2, 10, 11, 13, 100} {
		want := strings.TrimSpace(rubyEval(t, bin,
			"print Prime.each("+strconv.FormatInt(ub, 10)+").to_a.join(\",\")"))
		var ps []*big.Int
		Each(ub, func(p *big.Int) bool { ps = append(ps, p); return true })
		if got := joinInts(ps); got != want {
			t.Errorf("Each(%d)\n go: %q\nmri: %q", ub, got, want)
		}
	}
}

// TestOraclePrimeDivision cross-checks PrimeDivision against Prime.prime_division,
// including the negative-sign [-1,1] convention.
func TestOraclePrimeDivision(t *testing.T) {
	bin := rubyBin(t)
	nums := []int64{1, -1, 2, -2, 12, -12, 360, 100, 7919, 1000000, -1000000, 1234567}
	for _, n := range nums {
		want := strings.TrimSpace(rubyEval(t, bin,
			"print Prime.prime_division("+strconv.FormatInt(n, 10)+").inspect"))
		got := inspectPairs(PrimeDivision(big.NewInt(n)))
		if got != want {
			t.Errorf("prime_division(%d)\n go: %q\nmri: %q", n, got, want)
		}
	}
}

// TestOracleZeroDivision confirms MRI raises ZeroDivisionError for
// prime_division(0), matching this package's ZeroError panic.
func TestOracleZeroDivision(t *testing.T) {
	bin := rubyBin(t)
	out := rubyEval(t, bin,
		"begin; Prime.prime_division(0); print \"noraise\"; rescue ZeroDivisionError; print \"raised\"; end")
	if strings.TrimSpace(out) != "raised" {
		t.Errorf("MRI prime_division(0) did not raise ZeroDivisionError: %q", out)
	}
	// And the package mirrors it.
	defer func() {
		if r := recover(); r == nil {
			t.Error("PrimeDivision(0) should panic like MRI")
		}
	}()
	PrimeDivision(big.NewInt(0))
}

// TestOracleIntFromPrimeDivision cross-checks Int against
// Prime.int_from_prime_division for the integer reconstructions.
func TestOracleIntFromPrimeDivision(t *testing.T) {
	bin := rubyBin(t)
	for _, n := range []int64{12, -12, 1, -1, 360, 100, 7919} {
		want := strings.TrimSpace(rubyEval(t, bin,
			"print Prime.int_from_prime_division(Prime.prime_division("+
				strconv.FormatInt(n, 10)+"))"))
		got := Int(PrimeDivision(big.NewInt(n))).String()
		if got != want {
			t.Errorf("int_from_prime_division(%d)\n go: %q\nmri: %q", n, got, want)
		}
	}
}

// joinInts renders []*big.Int as a comma-joined string (MRI Array#join order).
func joinInts(ps []*big.Int) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.String()
	}
	return strings.Join(parts, ",")
}

// inspectPairs renders [][2]*big.Int the way Ruby's Array#inspect prints
// prime_division output, e.g. [[2, 2], [3, 1]].
func inspectPairs(ps [][2]*big.Int) string {
	if len(ps) == 0 {
		return "[]"
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = "[" + p[0].String() + ", " + p[1].String() + "]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
