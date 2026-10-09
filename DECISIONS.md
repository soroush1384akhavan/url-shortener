# Design Decisions

## Dependencies

- `github.com/swaggo/http-swagger` — used only to expose Swagger UI for manual API inspection during development. It is not part of the URL-shortening domain logic.
- GORM and the PostgreSQL driver are used for the persistent store in Part 4.

---

## Part 1 — Core URL Shortener

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

### Concurrency

`MemoryStore` uses `sync.RWMutex`.

Read-only operations such as `FindByURL` and `FindByCode` use `RLock`, while mutating operations use the exclusive `Lock`.

The duplicate check, collision check, and insertion are kept inside the same exclusive critical section.

Lock ownership remains inside the store; service and HTTP layers do not manipulate mutexes directly.


### Package layout

```text
cmd/
└── server/
    └── main.go

internal/
├── apperr/
├── domain/
├── httpapi/
├── link/
├── shortcode/
└── store/
```

Responsibilities are split as follows:

- `domain` — core `ShortLink` model
- `apperr` — shared sentinel errors
- `link` — validation, normalization, service orchestration, consumer-side interfaces
- `store` — concrete storage implementations
- `shortcode` — Base62 code generation
- `httpapi` — HTTP handlers, DTOs, routing, HTTP error mapping, Swagger annotations
- `cmd/server` — composition root and runtime wiring

### Testing

Part 1 tests cover validation, normalization, idempotency, collisions, retry exhaustion, concurrent shortening, redirect behavior, HTTP error mapping, and routing.

AI assistance was used as a consultant for focused questions about Go interfaces, package boundaries, error wrapping, concurrency, testing, and design trade-offs. Implementation and integration were performed in the repository.

---

## Part 2 — Abstraction and Metadata

Part 2 reused abstractions already introduced in Part 1, so only a small refactor was required.

### Metadata endpoint

Part 2 adds:

```text
GET /api/v1/links/{code}
```

A successful response contains the original URL and creation timestamp.

Example:

```json
{
  "url": "https://example.com/",
  "created_at": "2026-01-02T03:04:05Z"
}
```

A successful lookup returns `200 OK`, unknown links return `404 Not Found`, and unexpected failures return `500 Internal Server Error`.

A dedicated response DTO is used rather than serializing the domain object directly.

### Store abstraction

The shortening service consumes a `Store` interface instead of depending on `MemoryStore`.

Conceptually:

```text
ShortenerService
      |
      v
 Store interface
      ^
      |
 MemoryStore / PostgresStore
```

The interface is defined near the consumer. This keeps the service independent from concrete persistence technology and enabled Part 4 to add PostgreSQL without rewriting the business logic.

### Error semantics

Higher layers use:

```go
errors.Is(err, target)
```

for sentinel/category errors and `errors.As` when identifying a concrete error type such as `*http.MaxBytesError`.

Wrapping with `%w` preserves the original error chain.

### Metadata consistency

Because duplicate shortening returns the existing `ShortLink`, both the code and original `CreatedAt` remain stable.

The metadata endpoint therefore reports the original creation time rather than the time of the most recent request.

### Testing

Metadata tests cover:

- successful lookup
- content type
- correct URL and timestamp
- direct and wrapped `ErrNotFound`
- unexpected service errors
- invalid code lengths
- wrong method
- router registration

AI assistance was used as a consultant for error handling and test design concepts.

---

## Part 3 — Performance and Measurement

### HTTP server timeouts

The application uses an explicit `http.Server` with:

```text
ReadHeaderTimeout: 2s
ReadTimeout:       5s
WriteTimeout:      5s
IdleTimeout:       30s
```

These limits prevent slow or idle clients from holding server resources indefinitely.

### Graceful shutdown

The application listens for `SIGINT` and `SIGTERM` using `signal.NotifyContext`.

On shutdown, it calls `Server.Shutdown` with a 10-second timeout so the server stops accepting new connections while giving in-flight requests a bounded window to finish.

`http.ErrServerClosed` is treated as expected during normal shutdown.

### Locking choice

The in-memory store uses `sync.RWMutex` because the expected workload is strongly read-heavy.

`FindByCode` and `FindByURL` use shared read locking.

`SaveIfNotExist` and other mutations use an exclusive lock.

This matches the assumed workload of approximately 30,000 reads per second versus 300 writes per second.

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

### Memory profiling

The `alloc_space` profile showed that the main allocation sources were approximately:

```text
net/url.parse                      ~28.2%
crypto/rand.Int                    ~24.1%
MemoryStore.SaveIfNotExist         ~18.9%
math/big.nat.make                  ~10.5%
domain.NewShortLink                 ~5.2%
fmt.Sprintf                         ~4.1%
```

`fmt.Sprintf` is partly benchmark overhead because benchmark inputs intentionally use different URLs.

No in-memory eviction policy is implemented. While memory is the authoritative store, evicting a record could break the invariant:

```text
same normalized URL -> same short code
```

After Part 4, persistent storage can remain authoritative while memory or a distributed cache can later be introduced as a disposable acceleration layer.

### Testing server startup

I kept main small and moved the startup logic into run. I also moved storage selection into newStore.

This lets the tests call these functions directly and check the startup behavior and storage configuration.

After adding these tests, total statement coverage reached 94.0%. The main function itself is not directly covered, but run has 97.9% coverage and newStore has 55.6%.


AI assistance was used as a consultant for benchmark structure, `RunParallel`, `pprof`, and interpretation of `alloc_space` versus `inuse_space`.

---

## Part 4 — Persistence

### Storage choice

The durable storage implementation uses PostgreSQL with GORM.

The application supports two backends:

```text
memory
postgres
```

The backend is selected with:

```bash
go run ./cmd/server -storage=memory
go run ./cmd/server -storage=postgres
```

For PostgreSQL, the DSN is read from the `DATABASE_URL` environment variable rather than being hard-coded in the application.

PostgreSQL is run locally through Docker Compose with a persistent Docker volume.

PostgreSQL was chosen because it provides durable storage, database-level concurrency control, unique constraints, atomic updates, and a natural path toward multi-instance deployment.

### Store interface and context propagation

Storage operations accept `context.Context`.

The request flow is:

```text
HTTP request
    ↓
r.Context()
    ↓
Handler
    ↓
Service
    ↓
Store
    ↓
PostgreSQL / MemoryStore
```

The PostgreSQL store uses GORM's `WithContext(ctx)` so database operations can observe request cancellation and deadlines.

The in-memory store accepts the same context through the shared interface even though its map operations do not currently need it.

### Schema and model

Persistent links are stored in the `short_links` table.

The model contains:

```text
code
normalized_url
url_hash
created_at
used_count
```

Important constraints are:

```text
code
    PRIMARY KEY

normalized_url
    NOT NULL

url_hash
    CHAR(10)
    NOT NULL

(url_hash, normalized_url)
    UNIQUE

created_at
    NOT NULL

used_count
    NOT NULL
    DEFAULT 0
```

`code` is the primary key because it is already the unique immutable identifier used on the redirect path.

### URL hash and indexing

The complete normalized URL remains the authoritative value.

A deterministic 10-character truncated hash is stored as `url_hash` and used as a compact lookup prefix.

Lookup uses:

```sql
WHERE url_hash = ?
  AND normalized_url = ?
```

The truncated hash is not treated as the identity of a URL because collisions are theoretically possible.

The unique constraint is therefore:

```text
UNIQUE(url_hash, normalized_url)
```

rather than a uniqueness constraint on `url_hash` alone.

A separate index on only `url_hash` was not retained because the composite B-tree index already supports lookups by its leading column and an extra index would add write and storage overhead.

### Migrations

The current implementation uses GORM `AutoMigrate`.

When the PostgreSQL store is initialized, GORM ensures that the required table and constraints exist.

For this take-home assignment, this keeps setup simple.

For a production system with evolving schemas, explicit versioned migrations would be preferable because migration order and rollout can be controlled and reviewed.

### Idempotency across restarts

The Part 1 rule remains:

```text
same normalized URL -> same short code
```

The service first calls `FindByURL`.

At the database level, `(url_hash, normalized_url)` is unique, so concurrent attempts to create the same normalized URL cannot create multiple persistent mappings.

`SaveIfNotExist` uses PostgreSQL conflict handling. If another request has already inserted the same normalized URL, the duplicate insert is not created and the existing row is returned.

Because this mapping is persisted, the rule survives application restart:

```text
same normalized URL
    ↓
same persisted row
    ↓
same short code
```

A persistence/restart integration test verifies this using a second `PostgresStore` instance connected to the same database.

### Short-code collisions

`code` is a primary key.

If a generated code is already assigned to another URL, PostgreSQL returns a unique-constraint violation.

The PostgreSQL store translates the relevant database error into `ErrCodeCollision`.

The service then retries code generation without depending directly on PostgreSQL-specific error details.

### Crash safety and atomicity

A successful shorten response is returned only after `SaveIfNotExist` succeeds.

Therefore, the application does not return `201 Created` before the durable store has accepted the mapping.

The persistent implementation relies on database constraints and PostgreSQL concurrency semantics rather than a process-local Go mutex.

This matters because a Go mutex cannot coordinate multiple application instances.

The main correctness guarantees are:

- `code` uniqueness is enforced by the primary key
- URL idempotency is enforced by the composite unique constraint
- concurrent duplicate insertion is handled by database conflict semantics
- PostgreSQL provides transactional locking/MVCC behavior for concurrent access

### `created_at`

`created_at` is generated using:

```go
time.Now().UTC()
```

It is persisted in PostgreSQL using a timestamp-with-time-zone column.

When a row is loaded, the stored timestamp is mapped back to the domain object instead of creating a new timestamp.

This preserves the original creation time across duplicate requests and application restarts.

### Domain/database mapping

The persistence model is intentionally separate from the domain model.

Conceptually:

```text
domain.Code          -> model.Code
domain.LongURL       -> model.NormalizedURL
hash(domain.LongURL) -> model.URLHash
domain.CreatedAt     -> model.CreatedAt
domain.UsedCount     -> model.UsedCount
```

This keeps persistence-specific fields such as `URLHash` out of the domain model.

### Usage counting

The persistent store supports incrementing a usage counter by an arbitrary amount:

```text
IncrementUsedCount(ctx, code, amount)
```

The PostgreSQL implementation performs an atomic update equivalent to:

```sql
UPDATE short_links
SET used_count = used_count + ?
WHERE code = ?;
```

This avoids a read-modify-write race.

The in-memory implementation performs the same increment while holding its write lock.

#### Optional asynchronous usage-counter branch

An additional branch, `feat/async-usage-counter`, contains an asynchronous usage-counting design.

This branch was separated intentionally from the baseline PostgreSQL implementation so it can be reviewed and merged independently.

The motivation is that usage counting is analytics-like work and should not reduce redirect availability or add a synchronous database write to the hot redirect path.

The asynchronous design uses:

```text
redirect
   ↓
enqueue code into buffered channel
   ↓
return 302 without waiting for a database counter update

background worker
   ↓
aggregate counts by code
   ↓
flush on threshold or timer
   ↓
IncrementUsedCount(ctx, code, amount)
```

The worker aggregates repeated redirects for the same code so many events can become one database increment.

For example:

```text
abc1234 -> 450 redirects
xyz7890 -> 120 redirects
```

can be persisted as two increments instead of 570 individual updates.

The queue is bounded. If it is full, usage events are dropped rather than blocking the redirect path.

A periodic flush prevents low-traffic events from remaining buffered indefinitely, while a threshold-based flush handles bursts efficiently.

Graceful shutdown drains pending events and performs a final flush after the HTTP server has stopped accepting new work.

The trade-off is intentional: usage counting becomes best-effort and eventually consistent. A process crash before a buffered event is flushed can lose that event, and a database flush failure can cause analytics under-counting. Redirect reliability is prioritized over exact analytics.


No new benchmark is required merely to document this branch. The existing Part 3 measurements remain valid for the baseline in-memory implementation. If the asynchronous branch is merged and a measured performance claim is added, a focused before/after redirect benchmark should be run rather than assuming the improvement.

### PostgreSQL integration tests

PostgreSQL integration tests no longer rely on a hard-coded DSN.

They read the test/database connection from environment configuration so the suite is not tied to one developer workstation or one fixed port.

If the required external database is unavailable, integration behavior should be handled explicitly rather than failing because of a machine-specific hard-coded connection string.

The test suite covers:

- PostgreSQL save and lookup
- lookup by code
- lookup by normalized URL
- URL idempotency
- short-code collision handling
- persisted usage counts
- missing-record behavior
- persistence through creation of a new store instance
- direct MemoryStore behavior
- MemoryStore idempotency and collision handling
- MemoryStore usage-count behavior

### Verification

The final project checks are:

```bash
go test ./...
go vet ./...
go test -race ./...
```

Coverage is generated with:

```bash
go test ./... -coverprofile=coverage
go tool cover -func=coverage
go tool cover -html=coverage -o coverage.html
```

Total statement coverage is above the required 70% threshold.

### Trade-offs

The truncated URL hash is a lookup optimization, not a source of truth. Exact normalized-URL comparison preserves correctness in the presence of hash collisions.

`AutoMigrate` was selected for simplicity, while explicit versioned migrations would be preferable in a production system.

The PostgreSQL implementation is durable and suitable as shared storage, but it is not yet a complete high-scale architecture. Caching, replicas, partitioning, and load balancing are separate scaling concerns.

The optional asynchronous usage-counter branch reduces coupling between redirect latency and counter writes, but it intentionally trades strict counter durability for redirect availability.

### AI assistance

AI assistance was used as a technical consultant for:

- PostgreSQL/GORM integration
- schema and indexing trade-offs
- context propagation
- PostgreSQL conflict handling
- atomic counter updates
- integration-test configuration
- coverage/debugging discussions
- the design of the optional asynchronous usage counter, including buffered channels, aggregation, periodic/threshold flushing, and shutdown behavior

The implementation and repository integration were performed and verified in the project.

---

## Final verification checklist

Before submission, the project should remain green under:

```bash
go test ./...
go vet ./...
go test -race ./...
```

The repository should also include:

- updated `README.md`
- this `DECISIONS.md`
- Docker Compose configuration for PostgreSQL
- environment-based database configuration
- total statement coverage of at least 70%
## Part 5 — Scaling the Service

The following sections describe how I would handle more traffic.
These are proposed improvements and are not implemented yet.

### Stateless applications and shared storage

If one copy of the application cannot handle all the requests, I would
run several copies behind a load balancer. The load balancer would
distribute requests between healthy application instances.

All instances would connect to the same PostgreSQL database and, if
caching is added, the same Redis instance. This would allow any instance
to handle any short link.

Important link data would stay in PostgreSQL instead of only in one
instance's memory. If an instance stops working, the others could still
read and serve the saved links.

This would spread the application workload, but the shared database
could still become overloaded. Running more instances would also
increase costs and make deployment more complicated.

### Write-path scaling: rate limiting

I would limit how often each client IP can call `POST /api/shorten`.
As a starting point, I would allow 60 requests per minute per IP and
adjust this limit after testing.

The limit would be checked before accessing PostgreSQL. Requests above
the limit would receive `429 Too Many Requests` with a `Retry-After`
header telling the client when to try again.

This would reduce repeated URL lookups and insert attempts. The limit
would apply only to shortening URLs, so users could still open existing
short links.

With multiple application instances, I would keep the rate-limit counts
in a shared Redis instance. Checking and updating a count would need to
happen atomically so concurrent requests cannot bypass the limit.
Otherwise, separate counters in each instance could allow more requests
than intended.

One downside is that users sharing the same public IP would also share
the limit. Requests from many different IPs could still overload the
database, so per-IP limiting would only be a first step.

### Read-path scaling: caching popular links

Some short links may be opened many times. Reading the same destination
from PostgreSQL for every request adds repeated database work.

I would use Redis to cache the `code -> longURL` mapping. All application
instances would use the same cache.

For each redirect request, the application would:

1. Look for the code in Redis.
2. If it exists, use the cached destination.
3. Otherwise, read the destination from PostgreSQL.
4. If the link exists, add it to Redis for later requests.

PostgreSQL would remain the main storage. Removing a cached entry would
not delete the saved link or change its code. The application could
load it from PostgreSQL again.

I would start with a cache lifetime, or TTL, of 60 seconds and adjust it
after testing. I would also set a memory limit and use a policy that
removes less frequently used entries when the cache becomes full.

This should reduce database reads, although accessing Redis still
requires a network request. If Redis is unavailable, the application
could fall back to PostgreSQL, which would then receive more traffic.

Redis would also add memory costs and another service to manage.
I would measure response times and how often requests find a cached
entry before deciding whether the improvement is worth the cost.

### CDN caching for redirects

For popular links, I would also configure a CDN to cache successful
`302 Found` redirect responses for 60 seconds.

When a response is cached, the CDN could return the redirect directly
without sending the request to the application. This would reduce
traffic reaching both the application and its storage services.

Caching redirects would need explicit CDN configuration and suitable
cache headers; I would not assume that every `302` response is cached
automatically.

The trade-off is that a cached response may become outdated. If editing
or blocking links is added, a changed or blocked link could still
redirect to the old destination until the cached response expires.

I would remove affected entries from both Redis and the CDN when a link
changes. A short TTL would help limit how long an old response remains
available if cache removal fails.

Requests served by the CDN would also bypass the application's usage
counter. Accurate visit counts would therefore need information from
the CDN as well.

A CDN would add costs and configuration work. The 60-second TTL is an
initial design choice, not a value confirmed by load testing.

## Part 6 — Production Habits

### Graceful shutdown

Graceful shutdown is implemented in the server.

The application listens for `SIGINT` and `SIGTERM` using
`signal.NotifyContext`. When a shutdown signal arrives, it calls
`server.Shutdown` with a 10-second timeout.

During shutdown, the server stops accepting new connections, closes
idle connections, and gives active requests time to finish.

If requests finish within the timeout, shutdown completes normally.
If the timeout expires, the application logs the shutdown error and
exits. Requests that have not finished may be interrupted.

`http.ErrServerClosed` is treated as an expected result of normal
shutdown rather than an application failure.

The scaling proposals in Part 5 are separate from this implemented
shutdown behavior. Other Part 6 features, such as an enforced rate
limit on link creation, are not claimed as implemented here.

### Logging and observability

The application currently uses Go's standard `log` package.

It logs basic operational events such as:

- server startup
- server errors
- graceful shutdown
- unexpected errors while shortening a URL
- unexpected errors while finding a link
- errors while incrementing the usage counter

The HTTP handlers return simple error messages to clients and log the detailed error on the server side. This helps with debugging without exposing internal details directly in the response.

The application should not log full destination URLs, query strings, passwords, tokens, or other sensitive values. In particular, URLs may contain private information in their query parameters, so future logging changes should log only the short code, the operation name, the HTTP status, and the error type.

PostgreSQL and GORM may also produce database error logs when a database operation fails. Database logging should be configured carefully in production so SQL statements or sensitive URL values are not written to application logs.

The current project does not yet expose a full metrics system. For a production deployment, I would add request counts, response status counts, request duration, database errors, and cache hit/miss counts. These metrics would help detect slow requests and database or cache problems without storing full URLs.

This logging policy is intended to make operational problems easier to investigate while reducing the chance of exposing sensitive URL data.

### Domain policy

The current validator accepts only URLs that use `http` or `https`
and contain a hostname. The application does not fetch or check the
destination URL before saving it.

This prevents unsupported schemes such as `ftp`, `file`, `javascript`,
and `data` from being stored. It also prevents the service from making
server-side requests to user-provided URLs.

A future production version should add a domain policy. It could block
localhost, private IP addresses, loopback addresses, and known dangerous
domains. A blocklist could also be used for domains that are reported
for phishing or abuse.

The policy should be checked before saving a new link. If a destination
is blocked, the server should return `400 Bad Request` and should not
write anything to the database.

This policy is not fully implemented yet. The current implementation
only validates the URL format and the allowed scheme.