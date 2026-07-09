# prime examples

Runnable pure-Ruby usage of the `prime` generator, primality test and integer factorisation, verified under the [rbgo](https://github.com/go-embedded-ruby) interpreter.

```sh
rbgo examples/prime_usage.rb
```

| File | Shows |
| --- | --- |
| `prime_usage.rb` | Generate the first n primes with `Prime.first` / `Prime.take`, enumerate primes up to a bound with `Prime.each`, test primality with `Prime.prime?` / `Integer#prime?`, and factorise with `Prime.prime_division` / `Integer#prime_division` and its inverse `Prime.int_from_prime_division`. |
