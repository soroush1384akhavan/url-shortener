# URL Shortener

A small HTTP URL-shortening service written in Go.

The current implementation supports:

- shortening `http` and `https` URLs
- returning the same short code for the same normalized URL
- redirecting short codes with `302 Found`
- metadata lookup for an existing short code
- concurrent access through an in-memory `RWMutex`-protected store
- explicit HTTP server timeouts
- graceful shutdown
- benchmarks and CPU/memory profiling

The current store is in-memory only. Persistence is not implemented yet, so data is lost when the process exits.

---

## Requirements

- Go 1.22+

---

## Run

From the repository root:

```bash
go run ./cmd/server
```

Default configuration:

```text
addr = :8080
base = http://localhost:8080
```

You can override them:

```bash
go run ./cmd/server -addr :8080 -base http://localhost:8080
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

Submitting the same URL again after normalization returns the same code and short URL.

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

The store maintains both:

```text
code -> ShortLink
normalized URL -> ShortLink
```

Before generating a new short code, the service checks the normalized URL index.

Therefore:

```text
same normalized URL -> same short code
```

Code generation only happens for new URLs.

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

If a newly generated code already exists for another URL, the service retries with a new code.

The maximum retry count is 20.

---

## Concurrency

The in-memory store uses `sync.RWMutex`.

Read operations use `RLock`:

```text
FindByCode
FindByURL
```

Writes use the exclusive lock:

```text
SaveIfNotExist
```

The expected workload is read-heavy, so `RWMutex` allows multiple redirect/metadata lookups to proceed concurrently.

The idempotency check, collision check, and map insertion are performed atomically inside `SaveIfNotExist`.

---

## HTTP server timeouts

The server is configured with:

```text
ReadHeaderTimeout: 2s
ReadTimeout:       5s
WriteTimeout:      5s
IdleTimeout:       30s
```

These values are intended to prevent slow clients from holding server resources indefinitely.

The API only handles small JSON bodies and small responses, so normal requests are expected to finish well within these limits.

---

## Graceful shutdown

The process listens for `SIGINT` and `SIGTERM`.

On shutdown, the server stops accepting new connections and gives in-flight requests up to 10 seconds to finish.

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
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

---

## Benchmarks

Run all benchmarks with allocation statistics:

```bash
go test -bench=. -benchmem ./...
```

Example results from Windows/amd64:

```text
BenchmarkShortenNewURL-20       645469    1987 ns/op    909 B/op    26 allocs/op
BenchmarkRedirect-20            563094    1834 ns/op   6691 B/op    24 allocs/op
```

The raw in-memory lookup is substantially cheaper:

```text
BenchmarkGetByCode-20         81780079      13.08 ns/op      0 B/op     0 allocs/op
```

The redirect benchmark includes HTTP handler work and `httptest` request/response allocation, so it should not be interpreted as pure store lookup cost.

---

## Profiling

CPU profiling was performed against the new-URL shortening benchmark.

Example:

```bash
go test ./internal/link -run=^$ -bench=BenchmarkShortenNewURL -cpuprofile=cpu.out
go tool pprof -top cpu.out
```

One profiling insight was that the main create-path CPU costs were random Base62 code generation, store insertion/map work, and URL parsing/normalization.

Approximately:

```text
Base62Generator.GenerateCode     ~27.5% cumulative CPU
MemoryStore.SaveIfNotExist       ~20.6% cumulative CPU
net/url.Parse                    ~11.9% cumulative CPU
NormalizeURL                     ~11.3% cumulative CPU
```

Locking itself was not the dominant CPU cost in this benchmark.

Memory allocation profiling also showed that the largest allocation sources were URL parsing, cryptographic random generation, and growth of the in-memory store.

---

## In-memory capacity

No eviction policy is currently implemented.

The in-memory store is the authoritative source of truth, so removing an entry would lose the mapping and could break the guarantee that the same normalized URL always returns the same short code.

A memory cap can be reconsidered after persistent storage is introduced, when memory can act as a cache instead of the source of truth.

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

Responsibilities are separated so the HTTP layer depends on service interfaces, while storage and domain logic remain independent of HTTP concerns.

---

## Current limitations

The current implementation is single-process and uses an in-memory store.

This means:

- all data is lost on restart
- multiple application instances cannot share mappings
- memory usage grows as new links are created
- persistent storage is not yet available

Persistence is planned as the next implementation phase.
