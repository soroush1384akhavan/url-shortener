# Design decisions

## Dependencies

- `github.com/swaggo/http-swagger` — used only to expose Swagger UI for manual API inspection during development. It is not required by the URL-shortening domain itself.
- Any other direct Swaggo module that appears in `go.mod` should also remain documented here because the assignment requires every non-stdlib/non-allowed dependency to be explained.

## Part 1

### URL normalization

URLs are validated before storage. Idempotency is based on the normalized representation, not on the raw request string.

The current normalization rules are:

- trim surrounding whitespace
- lowercase the scheme
- lowercase the host
- remove default port `:80` for HTTP
- remove default port `:443` for HTTPS
- preserve non-default ports
- ensure an empty path becomes `/`
- remove `.` and `..` path segments
- remove fragments
- remove an empty query marker
- preserve meaningful trailing slashes

For example, these are intentionally treated as different URLs:

```text
https://example.com/path
https://example.com/path/
```

This strategy is conservative: it canonicalizes obvious representation differences without changing path/query semantics that may be meaningful to the destination server.

### URL validation

The validator requires a URL to:

- be non-empty after trimming whitespace
- stay within the configured maximum URL length
- use only `http` or `https`
- contain a hostname
- not contain user information

The application never HTTP-fetches the destination during validation. Validation is syntactic only, so the shortener does not become a server-side fetcher.

Validation is abstracted behind a `Validator` interface. The current implementation is `URLValidator`.

Invalid URL failures wrap the sentinel `ErrInvalidURL` using `%w`, allowing callers to use `errors.Is` without depending on error-message text.

The maximum-length rule and rejection of userinfo are additional defensive policies beyond the minimum Part 1 requirements.

### Idempotency

The same normalized URL must always return the same short code.

Before generating a code, the service calls `FindByURL`. If the normalized URL already exists, the existing `ShortLink` is returned and the generator is not called.

The in-memory store therefore keeps two indexes:

```text
normalized URL -> *domain.ShortLink
code           -> *domain.ShortLink
```

The URL index exists primarily for idempotency; the code index exists for redirect lookup and collision detection.

Idempotency is also preserved under concurrency. Even if two goroutines both miss the initial read, `SaveIfNotExist` performs the final duplicate check and insertion under the same exclusive lock. Only one link is stored and all callers receive the same stored link/code.

### Domain model

`ShortLink` was moved from `internal/link` into `internal/domain`.

It contains:

- `Code`
- `LongURL`
- `CreatedAt`

`CreatedAt` is assigned when the link is first created and is stored in UTC.

Returning an existing `ShortLink` for duplicate URLs preserves the original code and creation time.

The `domain` package was introduced to keep the core model independent from service/storage concerns and to avoid an import cycle between the shortening service and the concrete store implementation.

### Error placement

Errors that need to be shared across package boundaries are kept separately from the domain model rather than being attached to `ShortLink`.

The project uses sentinel errors for conditions higher layers need to recognize, including:

```text
ErrInvalidURL
ErrNotFound
ErrCodeCollision
ErrCodeGenerationExhausted
```

Shared application-level errors may live in `internal/apperr`, while service-specific errors remain close to the service that owns them.

Errors are wrapped with `%w` when useful, and callers use `errors.Is` instead of string comparison.

### Storage

Part 1 uses `MemoryStore` with two maps:

```text
normalized URL -> *domain.ShortLink
code           -> *domain.ShortLink
```

`MemoryStore` lives in `internal/store`. The `Store` interface is defined on the consumer side near the shortening service so the service depends on an abstraction, not on the in-memory implementation.

The main write operation is `SaveIfNotExist`. It checks for an existing URL, checks for a code collision, and performs insertion while holding one exclusive lock.

A direct `Save` helper existed during development/testing, but it is not part of the normal shortening flow because it can bypass the invariants enforced by `SaveIfNotExist`. It should be removed if the final test setup no longer needs it.

### Code generation

Codes are generated only for URLs that are not already present in storage.

The current generator produces random 7-character Base62 codes using:

```text
0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz
```

A 7-character Base62 code has `62^7` possible values, approximately 3.5 trillion combinations.

Randomness comes from Go's `crypto/rand` package.

Generation lives in `internal/shortcode`. The service depends on a `Generator` interface rather than directly on `Base62Generator`, which also allows deterministic fake generators in tests.

The generator only produces candidate codes; it does not know whether a candidate is unique.

### Capacity assumptions and scale target

For capacity planning, I assume an eventual workload of approximately:

```text
30,000 reads / second
300 writes / second
```

This is roughly a 100:1 read-to-write ratio. The expected total data set is about:

```text
5,000,000,000 shortened URLs
```

This assumption influences several design choices. The read path is expected to dominate traffic, so redirect lookup should be optimized for fast indexed reads. The create path runs much less frequently, so it is acceptable for first-time URL creation to do a little more work for validation, normalization, code generation, and uniqueness checking.

With the current domain model, I estimate a raw logical record size of at most roughly 200 bytes for the code, normalized URL, timestamp, and basic record data.

At five billion records:

```text
5,000,000,000 × 200 bytes
≈ 1,000,000,000,000 bytes
≈ 1 TB raw data
```

This is acceptable as a high-level capacity estimate, but it is not a final database-size estimate. Real persistent storage will require additional space for indexes, row/page overhead, metadata, WAL/journaling, backups, and possibly replication. Those costs will be considered in Part 4.

### Code-space and collision probability

A 7-character Base62 code has:

```text
62^7 = 3,521,614,606,208
```

possible values, or about 3.52 trillion codes.

The expected total data set is about 5 billion URLs. At that scale, only:

```text
5,000,000,000 / 3,521,614,606,208
≈ 0.00142
≈ 0.142%
```

of the available code space is occupied.

There are two useful collision probabilities to distinguish.

If random codes were generated **without checking whether a code already exists**, the birthday paradox makes collisions likely much earlier than the namespace-size number alone suggests. The probability of having seen at least one collision reaches roughly 50% after about:

```text
sqrt(2 × 62^7 × ln(2))
≈ 2.2 million generated codes
```

By the time the system reaches the assumed 5-billion-URL scale, the probability that at least one collision has occurred without collision checking is effectively 100%.

With collision checking enabled, the relevant probability for each new generated candidate is the fraction of the code space already occupied.

At the expected 5-billion-URL scale:

```text
collision probability per new candidate
≈ 0.142%
```

which is roughly one colliding candidate in every 704 attempts.

Because the implementation checks the candidate against storage and retries on `ErrCodeCollision`, the probability of failing all 20 attempts at the expected 5-billion-record scale is approximately:

```text
(0.00142)^20
≈ 1.1 × 10^-57
```

which is negligible.

This is the reason 7 Base62 characters are considered sufficient for the assumed scale. Even when the system has reached the planned 5 billion stored URLs, the namespace is still only about 0.142% occupied, so a random candidate is overwhelmingly likely to be free.

The create path is expected to run at only about 300 writes per second, so a small amount of extra work for uniqueness checking and the occasional retry is acceptable. The redirect/read path is much more latency-sensitive because it is expected to handle around 30,000 reads per second, so reads should remain optimized separately through indexed lookup and, later, caching.

### Why the public code is not a URL hash

I intentionally did not use a hash of the long URL as the public short-code generator.

A hash-based design can make code generation deterministic, but it does not remove the collision problem. If a hash is truncated to the same 7-character Base62 output, it still has only the same `62^7` public-code states. A full cryptographic hash has a much larger state space, but exposing enough of it would make the short URL much longer.

Using a URL hash directly as the public code would also couple the public identifier to normalization and hashing choices. Changing those choices later could change generated codes for the same URL.

The current design separates the concerns:

```text
public identifier:
random 7-character Base62 code

idempotency lookup:
normalized URL -> existing record
```

This keeps the external code short while allowing the storage layer to optimize URL lookup independently.

For Part 4 persistence, I plan to keep the normalized URL as the authoritative value and add an indexed fixed-size hash of the normalized URL for efficient database lookup. The hash is intended as an internal search/index key, not as the public short code.

The database must still verify the stored normalized URL when using the hash index, because even a hash index can theoretically collide. This approach combines a compact random public code with a fast fixed-width lookup key for the read/check path.

### Collision handling

Uniqueness is checked by the store because only the store knows the current set of assigned codes.

`SaveIfNotExist` checks the URL and candidate code while holding the same exclusive lock:

```text
lock
 |
 +-> URL already exists? ---- yes ---> return existing link
 |
 +-> code already exists? --- yes ---> ErrCodeCollision
 |
 +-> save both indexes
 |
unlock
```

The service handles `ErrCodeCollision` by generating another candidate and retrying.

Retries are bounded by:

```go
maxAttempts = 20
```

If all attempts fail, the service returns `ErrCodeGenerationExhausted` instead of retrying forever.

Collision behavior is tested with a deterministic fake generator, including both successful retry and exhausted-attempt cases.

### Concurrency and locking

Go HTTP requests may execute concurrently, so the shared in-memory maps must be synchronized.

`MemoryStore` uses `sync.RWMutex`.

Read-only methods such as `FindByURL` and `FindByCode` use `RLock` / `RUnlock`, allowing multiple concurrent readers.

Mutating operations use `Lock` / `Unlock`.

The important check-and-save sequence is performed inside one exclusive critical section. This prevents a check-then-act race where two goroutines both observe that a URL/code is absent and then create inconsistent state.

Lock ownership is entirely inside `MemoryStore`; service and HTTP code do not manipulate mutexes directly.

Concurrency tests cover:

- many goroutines shortening the same URL and receiving the same code
- many goroutines shortening different URLs without losing or mixing mappings

`go test -race ./...` passed under WSL/Linux with no reported data races.

### Shortening flow

```text
raw URL
   |
validate
   |
normalize
   |
FindByURL
   |
   +-> found: return existing link
   |
generate candidate Base62 code
   |
SaveIfNotExist
   |
   +-> collision: retry
   |
return stored link
```

Validation, normalization, generation, storage, and HTTP concerns are intentionally separated instead of being implemented inside one handler.

### Package layout

```text
cmd/
└── server/
    └── main.go

internal/
├── apperr/
├── domain/
│   └── short_link.go
├── httpapi/
├── link/
├── shortcode/
└── store/
```

Responsibilities:

- `domain` — core `ShortLink` model
- `apperr` — shared application-level sentinel errors where cross-package ownership is needed
- `link` — URL validation, normalization, `ShortenerService`, consumer-side interfaces, and orchestration
- `store` — concrete in-memory implementation, indexes, and synchronization
- `shortcode` — `Generator` abstraction and Base62 implementation
- `httpapi` — handlers, DTOs, route registration, HTTP error/status mapping, and Swagger annotations
- `cmd/server` — composition root that constructs and wires dependencies

### Dependency direction

The shortening service depends on abstractions it consumes:

```text
Validator
   ^
   |
ShortenerService -> Store
   |
   v
Generator
```

`MemoryStore` implicitly satisfies `Store`.

`Base62Generator` implicitly satisfies `Generator`.

The HTTP handler depends on a `link.Shortener` interface rather than on the concrete service type. `NewHandler` also accepts the interface, which makes handler tests able to use a small fake service.

Concrete implementations are created in `cmd/server/main.go` and injected into higher layers.

### HTTP API

#### `POST /api/shorten`

A successful request returns exactly `201 Created` with JSON containing `code` and `short_url`.

Submitting the same normalized URL again still returns `201 Created` and the same `code`/`short_url`.

Malformed JSON, missing/empty URLs, and invalid URLs return `400 Bad Request`.

Unexpected service failures are logged internally and exposed as a generic `500 Internal Server Error`.

The request body is bounded with `http.MaxBytesReader`; oversized bodies return `413 Request Entity Too Large`.

#### `GET /{code}`

A known code returns `302 Found` with the original long URL in the `Location` header.

Unknown codes return `404 Not Found`.

The handler also rejects obviously invalid code lengths outside the allowed 6–8 character range with `404`.

Unexpected lookup errors return `500 Internal Server Error`.

### Routing

The project uses Go's standard `net/http` package and Go 1.22+ method-aware `ServeMux` routing:

```text
POST /api/shorten
GET /{code}
```

Handlers still contain defensive method checks, even though normal routing already enforces methods.

Router-level tests exercise the actual `ServeHTTP` path, including a full shorten-then-redirect flow, so route registration itself is tested rather than bypassed.

### DTOs

The HTTP layer uses dedicated request/response structs instead of serializing the internal `ShortLink` model directly.

The Part 1 request exposes `url`.

The Part 1 response exposes `code` and `short_url`.

Internal fields such as `CreatedAt` and normalized `LongURL` are not exposed by the shorten endpoint.

### Server configuration

Runtime configuration is provided using command-line flags.

`-addr` controls the listen address and defaults to:

```text
:8080
```

`-base` controls the public base URL used to construct short URLs and defaults to:

```text
http://localhost:8080
```

These values are intentionally separate so the process can listen on an internal address while publishing links for an external host, such as behind a reverse proxy.

### Swagger / API documentation

Swagger was added only as a development convenience for manual API inspection.

Swagger annotations remain in the HTTP layer because they describe the transport contract rather than domain behavior.

Swagger is not required for Part 1 grading. Any non-stdlib Swagger modules present in `go.mod` are documented under `## Dependencies`.

### Testing strategy and final verification

Part 1 service tests cover:

- valid shortening
- URL normalization
- same URL returning the same stored link/code
- found/not-found lookups
- invalid URLs
- validator failures
- generator failures
- collision retry
- retry exhaustion
- concurrent duplicate shortening
- concurrent shortening of different URLs

HTTP tests use `httptest` and cover:

- successful shorten returning exactly `201`
- response JSON containing `code` and `short_url`
- HTTP-level idempotency
- malformed request bodies
- missing/empty URL values
- invalid URL handling
- oversized request bodies
- internal service errors
- redirect `302` and correct `Location`
- unknown/invalid codes
- router registration
- full shorten-then-redirect flow

A small fake service is used for handler error-path tests. A custom failing `http.ResponseWriter` is used to exercise the response-encoding failure branch.

Final Part 1 verification passed with:

```text
go test ./...
go vet ./...
go test -race ./...
```

Global statement coverage measured with:

```text
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

was:

```text
90.4%
```

which exceeds the required 70% threshold.

### AI assistance

AI was used only as a consultant for focused questions about Go concepts, package boundaries, interfaces, error wrapping, concurrency, `httptest`, race detection, and design tradeoffs.

The implementation and integration were performed manually.

AI also helped explain the idea of a custom failing `http.ResponseWriter` for exercising the JSON-encoding failure path; the final code was written and integrated manually.

No complete project module or complete handler implementation was generated for direct submission.



# Design decisions

## Part 2

### Part 2 started from a Part 1 architecture that was already prepared for it

A significant part of the Part 2 requirements had already been satisfied during Part 1.

This was intentional. Part 1 was implemented with package boundaries, interfaces, domain separation, and explicit error categories instead of putting all behavior directly inside the HTTP handlers.

Because of that, Part 2 did not require a large refactor. Most of the work was adding the metadata endpoint and verifying that the abstractions already introduced in Part 1 behaved correctly.

In particular, these Part 2 concerns were already handled during Part 1:

- the shortening service depended on a `Store` interface instead of a concrete `MemoryStore`
- the `Store` interface was defined close to the consumer
- the concrete in-memory implementation lived in the `store` package
- `ShortLink` already contained `CreatedAt`
- `ShortLink` had already been moved into the independent `domain` package
- `ErrInvalidURL` already existed as a recognizable sentinel error
- `ErrNotFound` already existed for failed code lookup
- errors were already wrapped with `%w` where extra context was needed
- higher layers already used `errors.Is` instead of comparing error strings
- idempotency was already implemented through normalized-URL lookup
- tests already used interfaces and fake dependencies where useful

As a result, Part 2 mostly extended the existing design instead of replacing it.

### Metadata endpoint

Part 2 adds:

```text
GET /api/v1/links/{code}
```

This endpoint returns metadata about an existing shortened link instead of redirecting the client.

The response shape is:

```json
{
  "url": "https://example.com/",
  "created_at": "2026-01-02T03:04:05Z"
}
```

A successful lookup returns `200 OK`.

An unknown code returns `404 Not Found`.

Unexpected service failures return `500 Internal Server Error`.

The metadata endpoint uses a dedicated response DTO, `GetMetadataResponse`, rather than serializing the domain model directly.

This keeps the HTTP contract separate from the internal `ShortLink` representation and allows the domain model to change without automatically changing the public API.

### Reuse of existing domain metadata

`ShortLink` already stored:

```text
Code
LongURL
CreatedAt
```

from Part 1.

Therefore Part 2 did not require a new persistence structure just to support metadata lookup.

`CreatedAt` is assigned when the link is first created and is stored in UTC.

When the same normalized URL is shortened again, the previously stored `ShortLink` is returned rather than creating a new one. This means the original creation timestamp is preserved.

The metadata endpoint therefore reports the creation time of the original short link rather than the time of the most recent duplicate shorten request.

### Store abstraction

The shortening service consumes a `Store` interface rather than depending directly on `MemoryStore`.

The interface is defined near the consumer rather than inside the implementation package.

Conceptually:

```text
ShortenerService
      |
      v
 Store interface
      ^
      |
 MemoryStore
```

This choice was already made in Part 1, so Part 2 could reuse it without refactoring the service.

The main benefit is that business logic does not depend on a specific storage implementation.

This will also make Part 4 persistence easier because a persistent store can implement the same abstraction without requiring the service to be rewritten around a database-specific type.

### Sentinel errors

Part 2 requires callers to be able to distinguish important application-level failures without depending on error-message text.

The project uses sentinel errors including:

```text
ErrInvalidURL
ErrNotFound
```

`ErrInvalidURL` represents invalid shortening input.

`ErrNotFound` represents a failed lookup when no link exists for the requested short code.

The HTTP layer maps these categories to transport-specific responses:

```text
ErrInvalidURL -> 400 Bad Request
ErrNotFound   -> 404 Not Found
```

Unexpected errors are treated as internal failures and returned as `500 Internal Server Error`.

The client is not given internal error details.

### Error wrapping with `%w`

When a lower-level error needs additional context, it is wrapped using `%w`.

Conceptually:

```text
ErrNotFound
    |
lookup failed: ErrNotFound
    |
higher-level context: lookup failed: ErrNotFound
```

The important property is that the original error remains part of the error chain.

This allows the code to preserve both a stable machine-checkable error category and additional human-readable context for debugging.

Using `%v` instead would only include the error text and would break the unwrap chain.

### `errors.Is`

Higher layers use:

```go
errors.Is(err, target)
```

instead of comparing error strings.

This allows a sentinel such as `ErrNotFound` or `ErrInvalidURL` to still be recognized even if one or more layers have wrapped it.

The metadata handler therefore correctly handles both a direct `ErrNotFound` and a wrapped error such as:

```text
lookup: ErrNotFound
```

as `404 Not Found`.

This behavior is covered by a test using a wrapped `ErrNotFound`.

### `errors.As`

`errors.As` is used when the code needs to identify an error by its concrete type rather than by sentinel identity.

For example, request-body size handling uses `*http.MaxBytesError`.

The code uses `errors.As` there because it wants to detect whether an error in the chain has that concrete type.

This is different from `errors.Is`, which is used for recognizable sentinel/category errors.

### Idempotency remains unchanged

Part 2 does not change shortening idempotency.

The same normalized URL still returns the same previously stored short link.

The lookup flow remains:

```text
raw URL
   |
validate
   |
normalize
   |
FindByURL
   |
   +-> found: return existing ShortLink
   |
generate candidate code
   |
save
```

Because the existing `ShortLink` is returned, both the code and `CreatedAt` remain stable across duplicate shorten requests.

Adding the metadata endpoint therefore did not introduce a second source of truth for link data.

### HTTP handler dependency

The HTTP handler depends on the `link.Shortener` interface rather than directly on `*ShortenerService`.

This was also introduced during Part 1.

It makes the handler easier to test because a small fake service can be injected without constructing the real store, validator, or generator.

The same fake service is reused for Part 2 tests to simulate successful metadata lookup, `ErrNotFound`, wrapped `ErrNotFound`, and unexpected internal errors.

### Metadata endpoint tests

Part 2 adds tests for the new endpoint.

The successful metadata test verifies:

- `200 OK`
- `Content-Type: application/json`
- the code passed to the service is correct
- the returned `url` is correct
- `created_at` is serialized as a valid RFC3339-compatible timestamp
- the returned timestamp matches the stored `CreatedAt`

Error-path tests cover:

- wrong HTTP method
- empty code
- code shorter than the accepted range
- code longer than the accepted range
- direct `ErrNotFound`
- wrapped `ErrNotFound`
- unexpected internal service error

A router-level test also sends:

```text
GET /api/v1/links/abc123
```

through the actual router.

This verifies that the endpoint is not only correct as a handler function, but is also registered correctly in routing.

### Verification

After the Part 2 changes, the project was verified with:

```text
go test ./internal/httpapi -v
go test ./...
go vet ./...
go test -race ./...
```

All tests passed.

The race-enabled test suite was run under WSL/Linux because the Windows race build required a C toolchain through `cgo`.

### Part 2 summary

Part 2 required only a relatively small amount of new implementation because the Part 1 design had already introduced most of the necessary abstractions.

The main new behavior in Part 2 was the metadata endpoint.

The rest of Part 2 primarily validated and reused design decisions that were already present:

```text
consumer-side Store interface
sentinel errors
error wrapping
errors.Is
domain metadata
idempotent storage
testable HTTP dependency injection
```

This was a deliberate result of keeping Part 1 modular and avoiding unnecessary coupling between HTTP, business logic, storage, and domain types.




# Design decisions

## Part 3

### Part 3 focus

Part 3 focuses on production-oriented HTTP server behavior, concurrency choices, and performance measurement.

A large part of the concurrency foundation was already implemented in Part 1:

- `MemoryStore` already used `sync.RWMutex`
- read operations already used `RLock`
- write operations already used `Lock`
- concurrent same-URL and different-URL tests already existed
- `go test -race ./...` had already passed

Because of that, Part 3 mainly added:

- explicit HTTP server timeouts
- benchmark coverage for read and write paths
- parallel benchmark coverage
- performance observations based on measured results

### HTTP server timeouts

Instead of relying only on the convenience form of `http.ListenAndServe`, the application now constructs an explicit `http.Server`.

The configured values are:

```text
ReadHeaderTimeout: 2s
ReadTimeout:       5s
WriteTimeout:      5s
IdleTimeout:       30s
```

The purpose of these settings is to prevent slow or idle clients from holding server resources indefinitely.

`ReadHeaderTimeout` limits how long a client may take to send HTTP headers.

`ReadTimeout` limits how long the server spends reading a request.

`WriteTimeout` limits how long the server spends writing a response.

`IdleTimeout` limits how long an idle keep-alive connection may remain open between requests.

These timeouts are server-level transport settings rather than handler business logic.

### Graceful shutdown

While adding explicit `http.Server` configuration, graceful shutdown support was also introduced.

The server runs in a goroutine while the main goroutine waits for either:

```text
server failure
or
SIGINT / SIGTERM
```

Shutdown signals are observed through `signal.NotifyContext`.

When a shutdown signal is received, the application calls:

```text
server.Shutdown(...)
```

with a 10-second timeout context.

This stops accepting new requests and gives active requests a limited window to complete before the process exits.

`http.ErrServerClosed` is treated as an expected result of normal shutdown rather than as a server failure.

Graceful shutdown is not the main Part 3 requirement, but it was added naturally while moving from `http.ListenAndServe` to an explicit `http.Server`.

### Mutex choice

The in-memory store uses:

```go
sync.RWMutex
```

instead of a plain `sync.Mutex`.

The expected workload is strongly read-heavy:

```text
approximately 30,000 reads / second
approximately 300 writes / second
```

or about a 100:1 read-to-write ratio.

Read operations such as:

```text
FindByURL
FindByCode
```

use `RLock` / `RUnlock`.

Mutating operations use `Lock` / `Unlock`.

The most important write operation, `SaveIfNotExist`, keeps the duplicate check, collision check, and insertion inside the same exclusive critical section.

The reason for using `RWMutex` is that multiple readers can proceed concurrently while writes remain exclusive.

This matches the expected access pattern, where redirect and lookup traffic is much more common than link creation.

### Benchmark strategy

Benchmarks were added in the `link` package to measure the main service paths.

The benchmark set currently includes:

```text
BenchmarkShortenNewURL
BenchmarkShortenExistingURL
BenchmarkShortenParallel
BenchmarkGetByCode
BenchmarkGetByCodeParallel
```

Each benchmark measures a different behavior.

`BenchmarkShortenNewURL` measures the full create path for a new URL, including validation, normalization, random code generation, and insertion.

`BenchmarkShortenExistingURL` measures the idempotent fast path where the normalized URL is already present.

`BenchmarkShortenParallel` measures concurrent creation of new URLs and exposes write-lock contention and concurrent service overhead.

`BenchmarkGetByCode` measures the sequential read path used for code lookup.

`BenchmarkGetByCodeParallel` measures concurrent read lookup and helps evaluate the effect of `RWMutex` under a read-heavy workload.

Benchmarks report both execution time and allocations.

### Benchmark environment

The first benchmark run was measured on:

```text
OS:   Windows
Arch: amd64
CPU:  12th Gen Intel(R) Core(TM) i7-12700H
```

The benchmark command was:

```text
go test '-bench=.' -benchmem '-run=^$' ./internal/link
```

Absolute benchmark numbers depend on CPU, operating system, Go version, scheduler behavior, and current machine load.

The main value of the results is therefore the relative behavior between paths rather than treating the raw nanosecond values as universal production latency.

### Benchmark results

The measured results were:

```text
BenchmarkShortenNewURL-20
645469 iterations
1987 ns/op
909 B/op
26 allocs/op

BenchmarkShortenExistingURL-20
2838927 iterations
416.4 ns/op
336 B/op
3 allocs/op

BenchmarkShortenParallel-20
554614 iterations
2561 ns/op
1004 B/op
28 allocs/op

BenchmarkGetByCode-20
81780079 iterations
13.08 ns/op
0 B/op
0 allocs/op

BenchmarkGetByCodeParallel-20
24400410 iterations
48.08 ns/op
0 B/op
0 allocs/op
```

### Performance observations

The most important observation is that the read path is substantially cheaper than the write path.

`GetByCode` completed in approximately:

```text
13.08 ns/op
```

with:

```text
0 B/op
0 allocs/op
```

in the sequential benchmark.

This is a strong result for the expected redirect hot path because code lookup does not allocate memory in the measured in-memory implementation.

The parallel read benchmark completed in approximately:

```text
48.08 ns/op
```

with the same:

```text
0 B/op
0 allocs/op
```

The concurrent version is slower than the sequential version, which is expected because of synchronization and goroutine scheduling overhead.

However, concurrent lookup remains much cheaper than any shortening path.

This supports the choice of `RWMutex` for the current read-heavy in-memory design.

### New URL creation cost

Creating a new short link measured approximately:

```text
1987 ns/op
909 B/op
26 allocs/op
```

This path performs more work than lookup:

```text
validation
normalization
FindByURL
random Base62 generation
ShortLink creation
collision-safe insertion
```

Therefore a higher cost and additional allocations are expected.

This is acceptable for the assumed workload because new link creation is expected to be much less frequent than redirects.

### Existing URL fast path

Shortening an already-known URL measured approximately:

```text
416.4 ns/op
336 B/op
3 allocs/op
```

This is much cheaper than shortening a new URL.

The difference confirms that idempotency provides a useful fast path:

```text
validate
normalize
FindByURL
return existing ShortLink
```

The generator and write path are skipped.

This also supports the Part 1 decision to maintain a direct normalized-URL index instead of scanning stored links.

### Parallel write behavior

Parallel new-URL shortening measured approximately:

```text
2561 ns/op
1004 B/op
28 allocs/op
```

This is slower than the sequential new-URL path.

That is expected because writes eventually require an exclusive lock in `SaveIfNotExist`.

With concurrent writers, lock contention and goroutine scheduling add overhead.

This behavior is acceptable for the assumed workload because writes are expected to be only a small fraction of total traffic.

The result also reinforces the design goal of keeping the redirect/read path independent from write-heavy operations.

### Read-heavy workload conclusion

The benchmark results are consistent with the workload assumptions documented earlier.

The system is expected to handle far more reads than writes.

The current in-memory design gives the read path these useful properties:

```text
direct map lookup
shared read locking
zero measured allocations
very low per-operation cost
```

The write path is more expensive but occurs much less frequently.

This makes `RWMutex` a reasonable choice for the current architecture.

### Future impact of persistent storage

The current benchmark results are for the in-memory implementation.

They should not be interpreted as final production latency after Part 4 introduces persistent storage.

Once database access is added, the read path will be dominated more by storage and network latency than by Go map lookup.

This is one reason a future cache for hot:

```text
code -> longURL
```

mappings is planned.

The benchmark still provides a useful baseline because it measures the application-level service and synchronization overhead before database I/O is introduced.

### AI assistance

AI was used as a consultant to explain Go benchmark structure and the `testing.B.RunParallel` / `testing.PB` API.

The benchmark implementation was written and integrated manually.

This assistance should be mentioned because `RunParallel` was a Go testing API that was not previously familiar during implementation.

### Current Part 3 status

Completed so far:

```text
explicit http.Server configuration
ReadHeaderTimeout
ReadTimeout
WriteTimeout
IdleTimeout
graceful shutdown
RWMutex design already in place
sequential shorten benchmarks
existing-URL benchmark
sequential code lookup benchmark
parallel shorten benchmark
parallel code lookup benchmark
benchmark allocation reporting
initial performance analysis
```

Remaining Part 3 work:

```text
run the benchmark suite on WSL/Linux for an additional baseline
perform the required profiling step and record at least one profiling observation
run final go test ./...
run final go vet ./...
run final go test -race ./...
```

# Part 3 — Performance & Measurement

## Scope

Part 3 focuses on measuring and hardening the current single-process, in-memory implementation before introducing persistent storage.

The main goals were:

- configure explicit HTTP server timeouts
- justify the locking strategy
- benchmark shorten and redirect/read paths
- profile CPU and memory behavior
- document whether an in-memory capacity/eviction policy should exist

---

## Locking choice

The in-memory store uses `sync.RWMutex`.

The expected workload is strongly read-heavy. I assume approximately:

- 30,000 redirect/read requests per second
- 300 shorten/write requests per second
- roughly a 100:1 read-to-write ratio

Redirects and metadata lookups only need to read existing mappings, while shortening a new URL may update both the code index and the normalized-URL index.

`FindByCode` and `FindByURL` therefore use `RLock`, which allows multiple concurrent readers.

`SaveIfNotExist` uses the exclusive `Lock`. The idempotency check, collision check, and insertion are kept inside the same exclusive critical section so concurrent requests cannot create inconsistent mappings.

I chose `RWMutex` instead of a plain `Mutex` because reads are expected to dominate writes.

The benchmark and profiling results support this choice: the read path is extremely cheap, while most of the create-path cost comes from URL processing, random code generation, allocation, and map work rather than lock overhead.

---

## Server timeouts

The HTTP server is configured explicitly with:

```text
ReadHeaderTimeout: 2s
ReadTimeout:       5s
WriteTimeout:      5s
IdleTimeout:       30s
```

### `ReadHeaderTimeout`

Limits how long a client may take to send request headers.

This protects the server from clients that slowly send headers and keep connections occupied unnecessarily.

### `ReadTimeout`

Limits how long the server may spend reading the full request, including its body.

The API only accepts small JSON requests, so a request that needs several seconds to arrive is considered abnormally slow.

### `WriteTimeout`

Limits how long the server may spend writing a response.

Responses from this service are small JSON payloads or redirects, so normal responses should finish well below this value.

### `IdleTimeout`

Limits how long an HTTP keep-alive connection may stay idle between requests.

A client that exceeds the configured timeout may have its connection terminated instead of being allowed to consume server resources indefinitely.

---

## Graceful shutdown

While moving from `http.ListenAndServe` to an explicit `http.Server`, graceful shutdown was also added.

The process listens for `SIGINT` and `SIGTERM` using `signal.NotifyContext`.

When shutdown is requested, the server stops accepting new connections and calls `Server.Shutdown` with a 10-second timeout so in-flight requests have a chance to finish.

Graceful shutdown is not required for the core Part 3 performance requirement, but it fits naturally with the explicit `http.Server` setup and improves production behavior.

---

## Benchmarks

Benchmarks were added for both shortening and redirect/read paths.

Example results from Windows/amd64:

```text
BenchmarkShortenNewURL-20          645469      1987 ns/op      909 B/op     26 allocs/op
BenchmarkShortenExistingURL-20    2838927       416.4 ns/op    336 B/op      3 allocs/op
BenchmarkShortenParallel-20         554614      2561 ns/op     1004 B/op     28 allocs/op
BenchmarkGetByCode-20             81780079        13.08 ns/op     0 B/op      0 allocs/op
BenchmarkGetByCodeParallel-20     24400410        48.08 ns/op     0 B/op      0 allocs/op
BenchmarkRedirect-20                563094      1834 ns/op     6691 B/op     24 allocs/op
```

Absolute benchmark values depend on hardware, operating system, Go version, and background system load, so the main value of the benchmarks is the relative comparison between paths.

### Benchmark observations

- Existing-URL shortening is much cheaper than shortening a new URL.
- This is expected because idempotency allows the service to return the existing mapping without generating or storing a new code.
- `GetByCode` is extremely cheap in the current in-memory implementation and performs zero allocations in the measured benchmark.
- Parallel reads remain cheap despite synchronization overhead.
- Parallel creation is slower because writers need exclusive access to the store.
- The HTTP redirect benchmark is much more expensive than the raw store lookup because it includes handler work and `httptest` request/response allocation.
- Therefore the redirect benchmark should not be interpreted as pure store lookup cost.

---

## CPU profiling

CPU profiling was performed against `BenchmarkShortenNewURL`.

The most notable application-level cumulative CPU costs were approximately:

```text
Base62Generator.GenerateCode     ~27.5%
MemoryStore.SaveIfNotExist       ~20.6%
net/url.Parse                    ~11.9%
NormalizeURL                     ~11.3%
MemoryStore.FindByURL             ~7.5%
```

A significant portion of the code-generation cost comes from `crypto/rand.Int`, because public short codes are generated using cryptographically secure randomness.

The profile also showed runtime and garbage-collection work, which is consistent with the allocation-heavy nature of creating new records.

An important observation is that synchronization itself was not the dominant CPU cost.

The main create-path costs were:

- cryptographic random generation
- URL parsing and normalization
- map lookup, insertion, hashing, and growth
- memory allocation and garbage collection

This means there is currently no evidence that redesigning or removing the `RWMutex` would provide the most valuable optimization.

---

## Memory profiling

Memory profiling was also performed against `BenchmarkShortenNewURL`.

### Allocation profile

The `alloc_space` profile showed roughly 593 MB of cumulative allocations during the benchmark run.

The largest allocation sources were approximately:

```text
net/url.parse                         ~28.2%
crypto/rand.Int                       ~24.1%
MemoryStore.SaveIfNotExist            ~18.9%
math/big.nat.make                     ~10.5%
domain.NewShortLink                    ~5.2%
fmt.Sprintf                            ~4.1%
```

This indicates that allocation pressure in the create path mainly comes from:

- URL parsing
- cryptographic random-number generation
- growing/updating the in-memory maps
- creating `ShortLink` values
- generating unique benchmark URLs

`fmt.Sprintf` is partly benchmark overhead because the benchmark intentionally creates a different URL for each iteration.

The allocation attributed to `SaveIfNotExist` is also expected because `BenchmarkShortenNewURL` continuously grows the in-memory dataset.

This behavior does not by itself indicate a memory leak.

### In-use heap profile

The `inuse_space` snapshot was mostly dominated by Go runtime allocations and was less useful for understanding application-owned data.

The final snapshot showed only a few megabytes of live memory, mostly associated with runtime thread/goroutine infrastructure and time-zone initialization.

By the time the profile snapshot was captured, benchmark-owned service/store objects were no longer necessarily reachable, so garbage collection could reclaim them.

For this benchmark, `alloc_space` was therefore more useful than `inuse_space` for understanding where the application allocates memory.

---

## Profiling conclusion

The benchmark and profiling results are consistent.

The read path is extremely cheap in the current in-memory implementation.

The new-link creation path is more expensive because it performs:

- validation
- normalization
- cryptographic code generation
- memory allocation
- map lookup and insertion
- collision-safe synchronized writes

Given the expected read-heavy workload, further optimization of the less-frequent create path is not currently a priority.

When persistent storage is introduced, database and network latency are expected to dominate these in-memory micro-costs. Future optimization should therefore be based on profiling the persistent implementation instead of prematurely optimizing the current memory-only version.

---

## In-memory capacity and eviction

No maximum in-memory link count or eviction policy is implemented.

At this stage, the in-memory store is the authoritative source of truth.

Evicting an entry would therefore mean losing that mapping while the process is still running.

More importantly, eviction could break the idempotency guarantee:

```text
same normalized URL -> same short code
```

If a mapping were removed and the same URL were shortened again, the service could generate a different code.

For that reason, I chose not to implement FIFO, LRU, TTL, or another eviction policy while memory is the only storage layer.

Capacity management can be reconsidered after persistent storage is introduced. At that point, durable storage can remain authoritative while memory can be used as a disposable cache.

---

## Verification

Part 3 should be considered complete only while the following remain green:

```bash
go test ./...
go vet ./...
go test -race ./...
go test -bench=. -benchmem ./...
```

The race test is especially important because the store is accessed concurrently and correctness depends on the locking strategy.

---

## AI assistance

AI was used only as a consultant for understanding benchmark and profiling concepts.

Specifically, I asked for clarification about:

- the syntax and purpose of `b.RunParallel`
- CPU profiling with `-cpuprofile`
- `go tool pprof`
- `alloc_space` vs `inuse_space`
- interpretation of profiling output

The benchmark and application code were written and integrated manually.
