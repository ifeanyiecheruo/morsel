# Testing

## The rule

Every contract gets a test. If you add a method or change observable behaviour, add or update a test that would fail if the contract were broken. Run `make test` before pushing.

## Package placement

Use `package foo` (white-box) for tests that need access to unexported identifiers — internal helpers, middleware internals. Use `package foo_test` (black-box) for tests that exercise a public API from the outside. The rule of thumb: public interfaces get black-box tests; internal helpers get white-box tests.

## What to test

Test the contract, not the implementation. A test for `AmbientToken` on `LocalPlatform` asserts that it returns `"", nil` — the fact that it does so by returning literals is not the point. A test for `writeError` asserts the JSON shape and status code, not which line of code ran.

Avoid testing logging output, exact error messages that aren't part of a public contract, or internal state that isn't observable through the public interface.

## External dependencies

The test suite has no external dependencies. Tests run entirely in-process with no network, filesystem, or database access. `LocalPlatform` stubs are the only platform implementation used in tests.
