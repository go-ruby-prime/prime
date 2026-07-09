# frozen_string_literal: true
#
# Usage of Prime — the prime generator, primality test and integer
# factorisation added by `require "prime"`. Runs under go-embedded-ruby
# (rbgo); see examples/README.md.

require "prime"

# The generator: the first n primes, in ascending order.
p Prime.first(8)                                 # => [2, 3, 5, 7, 11, 13, 17, 19]
p Prime.take(5)                                  # => [2, 3, 5, 7, 11]

# Enumerate every prime p <= ubound (as an Array, or via a block).
p Prime.each(20).to_a                            # => [2, 3, 5, 7, 11, 13, 17, 19]
Prime.each(10) { |x| print x, " " }              # => 2 3 5 7
puts

# Primality — Prime.prime? and the Integer#prime? core extension agree, and
# even Carmichael numbers (561) are correctly rejected.
p Prime.prime?(97)                               # => true
p Prime.prime?(561)                              # => false
p 100.prime?                                     # => false

# Factorisation into [prime, exponent] pairs, and its exact inverse.
p Prime.prime_division(360)                      # => [[2, 3], [3, 2], [5, 1]]
p 360.prime_division                             # => [[2, 3], [3, 2], [5, 1]]
p Prime.int_from_prime_division([[2, 3], [3, 2], [5, 1]])  # => 360
