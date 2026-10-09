# URL Shortener

A small HTTP URL-shortening service written in Go.

The current implementation supports:

- shortening `http` and `https` URLs
- returning the same short code for the same normalized URL
- redirecting short codes with `302 Found`
- metadata lookup for an existing short code
- an in-memory storage backend
- a persistent PostgreSQL storage backend using GORM
- persistence across application restarts
- atomic redirect usage counting
- concurrent access
- request-context propagation to storage operations
- explicit HTTP server timeouts
- graceful shutdown
- benchmarks and CPU/memory profiling

The storage backend can be selected at startup.

---

## Requirements

- Go 1.24+
- Docker / Docker Compose for the PostgreSQL backend

The in-memory backend does not require PostgreSQL.

---

## Run

### In-memory storage

From the repository root:

```bash
go run ./cmd/server -storage=memory
```

`memory` is the non-persistent backend. Data is lost when the application exits.

### PostgreSQL storage

Start PostgreSQL:

```bash
docker compose up -d
```

Set the database connection string.

PowerShell:

```powershell
$env:DATABASE_URL="host=127.0.0.1 user=urlshortener password=urlshortener dbname=urlshortener port=5434 sslmode=disable"
```

Linux/macOS:

```bash
export DATABASE_URL="host=127.0.0.1 user=urlshortener password=urlshortener dbname=urlshortener port=5434 sslmode=disable"
```

Then run:

```bash
go run ./cmd/server -storage=postgres
```

The PostgreSQL backend persists links and usage counts across application restarts.

The Docker Compose configuration uses a persistent Docker volume, so stopping
the application or PostgreSQL container does not remove stored data.

To stop PostgreSQL while preserving data:

```bash
docker compose down
```

Removing the Docker volume will also remove the persisted database data:

```bash
docker compose down -v
```

Use the latter only when the stored data is no longer needed.

---

## Configuration

Default server configuration:

```text
addr    = :8080
base    = http://localhost:8080
storage = memory
```

Example:

```bash
go run ./cmd/server \
  -addr :8080 \
  -base http://localhost:8080 \
  -storage=postgres
```

### Flags

`-addr`

Listen address for the HTTP server.

Example:

```bash
-addr :8080
```

`-base`

Base URL used when constructing the returned `short_url`.

Example:

```bash
-base http://localhost:8080
```

`-storage`

Selects the storage backend.

Supported values:

```text
memory
postgres
```

Example:

```bash
-storage=postgres
```

When `postgres` is selected, `DATABASE_URL` must be set.

---

## API

### Shorten a URL

```http
POST /api/shorten
Content-Type: application/json
```

Request:

```json
{
  "url": "https://example.com/path"
}
```

Example:

```bash
curl -s -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/path"}'
```

Example response:

```json
{
  "code": "a1B2c3D",
  "short_url": "http://localhost:8080/a1B2c3D"
}
```

Successful requests return:

```text
201 Created
```

Submitting the same URL again after normalization returns the same code and
short URL.

With PostgreSQL storage, this idempotency is preserved across application
restarts.

---

### Redirect

```http
GET /{code}
```

Example:

```bash
curl -i http://localhost:8080/a1B2c3D
```

For an existing code:

```text
302 Found
Location: https://example.com/path
```

For an unknown code:

```text
404 Not Found
```

A successful redirect increments the stored usage counter.

For PostgreSQL this increment is performed atomically inside the database.

---

### Link metadata

```http
GET /api/v1/links/{code}
```

Example:

```bash
curl -s http://localhost:8080/api/v1/links/a1B2c3D
```

Example response:

```json
{
  "url": "https://example.com/path",
  "created_at": "2026-01-02T03:04:05Z"
}
```

For an unknown code:

```text
404 Not Found
```

Metadata lookup itself does not count as a redirect and therefore does not
increment the usage counter.

---

## URL validation and normalization

Only `http` and `https` URLs are accepted.

The service does not perform an HTTP request to the submitted URL.

Current normalization behavior includes:

- trimming surrounding whitespace
- lowercasing scheme and host
- removing default port `80` for HTTP
- removing default port `443` for HTTPS
- preserving non-default ports
- removing fragments
- removing empty query strings
- normalizing dot-segments in paths
- treating an empty path as `/`
- preserving a meaningful trailing slash

For example:

```text
HTTPS://Example.com:443/page#fragment
```

normalizes to:

```text
https://example.com/page
```

`/path` and `/path/` are intentionally treated as different paths.

---

## Idempotency

Idempotency is based on the normalized URL:

```text
same normalized URL -> same short code
```

The service checks the store before generating a new short code.

### Memory backend

The in-memory store maintains indexes for:

```text
code -> ShortLink
normalized URL -> ShortLink
```

### PostgreSQL backend

The PostgreSQL store persists both the normalized URL and a shortened hash of
that URL.

URL lookup uses both:

```sql
WHERE url_hash = ?
  AND normalized_url = ?
```

The hash is only a lookup optimization. The normalized URL remains the
authoritative value because truncated hashes can theoretically collide.

The database enforces a unique constraint on:

```text
(url_hash, normalized_url)
```

This ensures that the same normalized URL cannot be stored twice.

Because the mapping is persisted, shortening the same URL after restarting the
application returns the same short code.

---

## Code generation

Short codes are 7-character Base62 strings using:

```text
a-z
A-Z
0-9
```

The generator uses `crypto/rand`.

The code space is:

```text
62^7 = 3,521,614,606,208
```

If a newly generated code already exists for another URL, the service retries
with a new code.

The maximum retry count is 20.

With PostgreSQL, `code` is the primary key. A database uniqueness violation
for an already-used code is translated into the application's
`ErrCodeCollision`, allowing the service to retry without depending on
PostgreSQL-specific errors.

---

## Storage

The service depends on a `Store` abstraction rather than directly on a
concrete storage implementation.

Two implementations are available:

```text
MemoryStore
PostgresStore
```

This allows the storage backend to be selected without changing the shortening
business logic.

### MemoryStore

`MemoryStore` uses Go maps protected by `sync.RWMutex`.

It is fast but non-persistent.

### PostgresStore

`PostgresStore` uses PostgreSQL through GORM.

Persistent records contain:

```text
code
normalized_url
url_hash
created_at
used_count
```

`code` is the primary key.

The combination:

```text
(url_hash, normalized_url)
```

has a unique index used to preserve URL idempotency.

The database schema is initialized using GORM `AutoMigrate`.

For this take-home project, `AutoMigrate` keeps setup simple. In a larger
production system, explicit versioned migrations would be preferable.

---

## Persistence

The PostgreSQL backend is durable across application restarts.

A successful shorten request is returned only after the store operation
completes successfully.

The persisted mapping contains the original:

```text
code
normalized URL
created_at
used_count
```

Restarting the application and reconnecting to the same PostgreSQL database
therefore preserves the short-link mapping.

For example:

```text
shorten URL
    ↓
code = abc1234
    ↓
stop application
    ↓
restart application
    ↓
shorten same URL
    ↓
code = abc1234
```

Persistence behavior is covered by an integration test using multiple
`PostgresStore` instances connected to the same database.

---

## `created_at`

`created_at` is assigned when a new domain `ShortLink` is created.

The application generates it using UTC:

```go
time.Now().UTC()
```

PostgreSQL stores the timestamp using a timestamp-with-time-zone column.

When a persisted record is loaded, its original creation timestamp is restored
instead of generating a new timestamp.

Therefore `created_at` remains stable across duplicate shorten requests and
application restarts.

---

## Context propagation

Storage operations accept `context.Context`.

The request flow is:

```text
HTTP request
    ↓
r.Context()
    ↓
HTTP handler
    ↓
service
    ↓
store
```

The PostgreSQL implementation attaches the context to database operations using
GORM's `WithContext`.

This allows database work to observe request cancellation and context
deadlines.

The in-memory store accepts the same context through the shared interface,
although it does not currently require it for its map operations.

---

## Concurrency

### Memory backend

The in-memory store uses `sync.RWMutex`.

Read operations use `RLock`:

```text
FindByCode
FindByURL
```

Mutating operations use the exclusive lock:

```text
SaveIfNotExist
IncrementUsedCount
```

The idempotency check, collision check, and insertion are performed atomically
inside `SaveIfNotExist`.

### PostgreSQL backend

`PostgresStore` does not use a process-local Go mutex.

Concurrency and uniqueness are handled by PostgreSQL using database
constraints, locking, and MVCC.

This is important because a Go mutex would protect only one application
process and would not coordinate multiple server instances.


## Usage counter

Each stored link contains a usage counter.

A redirect increments the counter.

The PostgreSQL implementation performs an atomic database update equivalent to:

```sql
UPDATE short_links
SET used_count = used_count + 1
WHERE code = ?;
```

The implementation intentionally avoids a read-modify-write sequence such as:

```text
SELECT used_count
used_count++
UPDATE used_count
```

because concurrent requests could otherwise overwrite each other's increments.

The in-memory backend performs the increment while holding its exclusive
write lock.

---

### Optional asynchronous usage counting

An additional branch, `feat/async-usage-counter`, contains an asynchronous usage-counting implementation.

In that branch, redirects do not wait for a database counter update.

Instead, the redirect path records the short code in a bounded in-memory queue:

```text
redirect request
    ↓
find link
    ↓
enqueue usage event
    ↓
return 302
```

A background worker consumes queued events and aggregates them by short code.

For example:

```text
abc1234 -> 450 redirects
xyz7890 -> 120 redirects
```

can be persisted as two counter updates rather than hundreds of individual database writes.

The worker flushes aggregated counts when either:

```text
the configured threshold is reached
or
the periodic flush interval expires
```

Pending events are also flushed during graceful shutdown after the HTTP server has stopped accepting new requests.

The queue is intentionally non-blocking. If it becomes full, usage events may be dropped rather than delaying redirects.

This makes usage counting best-effort and eventually consistent:

- redirect availability is prioritized over analytics accuracy
- counters may temporarily lag behind actual redirects
- a process crash before a flush may lose buffered events
- failed flushes may result in under-counting

A durable queue could be introduced later if exact asynchronous accounting becomes a requirement.

The asynchronous implementation is kept on a separate branch so it can be reviewed and merged independently from the baseline PostgreSQL persistence implementation.

## HTTP server timeouts

The server is configured with:

```text
ReadHeaderTimeout: 2s
ReadTimeout:       5s
WriteTimeout:      5s
IdleTimeout:       30s
```

These values are intended to prevent slow clients from holding server resources
indefinitely.

The API only handles small JSON bodies and small responses, so normal requests
are expected to finish well within these limits.

---

## Graceful shutdown

The process listens for `SIGINT` and `SIGTERM`.

On shutdown, the server stops accepting new connections and gives in-flight
requests up to 10 seconds to finish.

---

## Tests

Run all tests:

```bash
go test ./...
```

Run static analysis:

```bash
go vet ./...
```

Run race detection:

```bash
go test -race ./...
```

Run coverage:

```bash
go test ./... -coverprofile=coverage
go tool cover -func=coverage
```

Generate an HTML coverage report:

```bash
go tool cover -html=coverage -o coverage.html
```

Total statement coverage is above the required 70% threshold.

The test suite includes coverage for:

- URL validation and normalization
- idempotent shortening
- code collisions and retry behavior
- HTTP handlers and routing
- concurrent in-memory access
- MemoryStore behavior
- PostgreSQL persistence
- PostgreSQL URL idempotency
- PostgreSQL code collisions
- persisted usage counting
- missing-record behavior
- persistence across new store instances

---

## PostgreSQL inspection

To open `psql` inside the Docker container:

```bash
docker exec -it url-shortener-postgres \
  psql -U urlshortener -d urlshortener
```

Useful commands:

```sql
\dt
```

```sql
\d short_links
```

```sql
SELECT * FROM short_links;
```

---

## Benchmarks

Run all benchmarks with allocation statistics:

```bash
go test -bench=. -benchmem ./...
```

Example results from Windows/amd64:

```text
BenchmarkShortenNewURL-20       645469     1987 ns/op     909 B/op     26 allocs/op
BenchmarkRedirect-20            563094     1834 ns/op    6691 B/op     24 allocs/op
```

The raw in-memory lookup is substantially cheaper:

```text
BenchmarkGetByCode-20         81780079       13.08 ns/op      0 B/op      0 allocs/op
```

The redirect benchmark includes HTTP handler work and `httptest`
request/response allocation, so it should not be interpreted as pure store
lookup cost.

These measurements describe the in-memory implementation. PostgreSQL-backed
operations include database and network overhead and therefore have different
performance characteristics.

---

## Profiling

CPU profiling was performed against the new-URL shortening benchmark.

Example:

```bash
go test ./internal/link \
  -run=^$ \
  -bench=BenchmarkShortenNewURL \
  -cpuprofile=cpu.out

go tool pprof -top cpu.out
```

One profiling insight was that the main create-path CPU costs were random
Base62 code generation, store insertion/map work, and URL
parsing/normalization.

Approximately:

```text
Base62Generator.GenerateCode     ~27.5% cumulative CPU
MemoryStore.SaveIfNotExist       ~20.6% cumulative CPU
net/url.Parse                    ~11.9% cumulative CPU
NormalizeURL                     ~11.3% cumulative CPU
```

Locking itself was not the dominant CPU cost in this benchmark.

Memory allocation profiling also showed that the largest allocation sources
were URL parsing, cryptographic random generation, and growth of the in-memory
store.

---

## In-memory capacity

No eviction policy is currently implemented for `MemoryStore`.

When the memory backend is selected, it is the authoritative source of truth.
Removing an entry would therefore lose the mapping and could break:

```text
same normalized URL -> same short code
```

The PostgreSQL backend does not have this limitation because mappings are
stored durably.

A future architecture could use an in-memory or distributed cache in front of
PostgreSQL while keeping persistent storage authoritative.

---

## Project structure

```text
cmd/
  server/
    main.go

internal/
  apperr/
  domain/
  httpapi/
  link/
  shortcode/
  store/
```

Responsibilities are separated so the HTTP layer depends on service
interfaces, while storage and domain logic remain independent of HTTP
concerns.

`cmd/server` acts as the composition root and selects the configured storage
backend.

---

## Current limitations

The application currently runs as a single server process.

The in-memory backend is not persistent and cannot share state across multiple
application instances.

The PostgreSQL backend provides durable shared storage, but the current
implementation does not yet include:

- a distributed cache for hot redirects
- read replicas
- database sharding
- load balancing across multiple application instances
- CDN caching
- distributed rate limiting

These are architecture/scaling concerns rather than requirements of the
current persistent implementation.