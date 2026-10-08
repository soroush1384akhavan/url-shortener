# Design decisions

## Part 1

### URL normalization

URLs are validated before they are stored.

The normalization strategy is intentionally conservative. The URL is parsed and normalized without changing path or query semantics.

For example, these URLs are currently treated as different URLs:

- `https://example.com/path`
- `https://example.com/path/`

This avoids modifying URLs in ways that could change their meaning.

Normalization is performed before looking up the URL in the store. This ensures that idempotency is based on the normalized representation rather than the raw input received by the HTTP API.

### URL validation

URLs are validated before normalization and storage.

The current validator requires a URL to:

- be non-empty
- be within the configured maximum URL length
- use either the `http` or `https` scheme
- contain a hostname
- not contain user information

The application does not make an HTTP request to the destination URL during validation.

Validation is separated from the shortening service through a `Validator` interface.

The current implementation uses `URLValidator`, while the interface allows the validation strategy to be replaced or extended later without changing the shortening service.

Validation errors are categorized using the sentinel error:

```go
ErrInvalidURL
```

Specific validation failures wrap `ErrInvalidURL` using `%w`. This allows the HTTP layer to identify invalid URLs with `errors.Is` while still preserving a more descriptive error message.

### Idempotency

The same normalized URL must always return the same short code.

Before generating a new code, the shortening service asks the store whether the normalized URL already exists.

If the URL already exists, the previously stored `ShortLink` is returned and the generator is not called.

Conceptually:

```text
raw URL
   ↓
validate
   ↓
normalize
   ↓
find existing URL
   ↓
found?
 ┌─────┴─────┐
yes          no
 ↓            ↓
return      generate
existing      code
link
```

The in-memory store maintains a URL-based index, allowing an existing URL to be found directly without scanning all stored links.

Idempotency is also preserved when multiple requests for the same URL are processed concurrently. The final duplicate check is performed atomically by the store before inserting a new link.

### Link model

A shortened link is represented by `ShortLink`.

It currently contains:

- `Code`
- `LongURL`
- `CreatedAt`

`CreatedAt` is assigned when a link is originally created and is stored in UTC.

When a duplicate URL is shortened, the previously stored `ShortLink` is returned instead of constructing a new one. This preserves the original metadata, including the creation time.

### Link storage

Part 1 uses an in-memory store.

The store maintains two indexes:

```text
normalized URL
     ↓
 ShortLink
```

and:

```text
code
 ↓
ShortLink
```

The URL index is primarily used for idempotency.

The code index is used for redirect lookup and collision detection.

The store implementation is kept separate from the shortening service.

The `Store` interface is defined close to the service that consumes it, while the concrete `MemoryStore` implementation is defined in the `store` package.

This allows the storage implementation to be changed later without coupling the shortening service directly to the in-memory implementation.

### Code generation

Short codes are generated only for URLs that do not already exist in the store.

The current implementation generates random 7-character Base62 codes.

The alphabet is:

```text
0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz
```

Each character therefore has 62 possible values.

A 7-character code provides:

```text
62^7
```

possible combinations, which is approximately 3.5 trillion.

Random values are generated using Go's `crypto/rand` package.

Code generation is implemented in the separate `shortcode` package.

The service depends on a `Generator` interface rather than directly depending on `Base62Generator`.

This makes the generation strategy replaceable and also allows deterministic generators to be introduced in tests.

The generator is responsible only for producing candidate codes. It does not know whether a code already exists in storage.

### Collision handling

Random code generation can theoretically generate a code that is already assigned to another URL.

Collision detection is performed by the store when attempting to insert a new link.

The store checks both the URL and the generated code while holding the same exclusive lock.

Conceptually:

```text
lock
 ↓
URL already exists?
 ├── yes → return existing link
 ↓ no
code already exists?
 ├── yes → ErrCodeCollision
 ↓ no
save link
 ↓
unlock
```

This prevents an existing code from being silently overwritten.

The shortening service handles `ErrCodeCollision` by generating another candidate code and trying again.

Retries are bounded using:

```go
maxAttempts = 20
```

If a unique code cannot be stored after the maximum number of attempts, the shortening operation fails instead of retrying indefinitely.

Collision detection is intentionally kept outside the generator because uniqueness depends on the current contents of storage.

### Concurrency and locking

HTTP requests may be processed concurrently by Go's HTTP server.

The in-memory store is therefore shared between multiple goroutines.

Go maps are not safe for concurrent reads and writes, so the memory store is protected by `sync.RWMutex`.

Read-only operations such as:

```text
FindByURL
FindByCode
```

use:

```go
RLock()
RUnlock()
```

This allows multiple readers to access the store concurrently.

Mutating operations use:

```go
Lock()
Unlock()
```

The important check-and-save operation is performed inside one exclusive critical section.

This avoids a check-then-act race where two goroutines could both observe that a URL or code does not exist and then attempt to insert conflicting data.

Locking is owned entirely by `MemoryStore`. The shortening service and HTTP handlers do not manipulate mutexes directly.

### Shortening flow

The current shortening flow is:

```text
raw URL
   ↓
validate
   ↓
normalize
   ↓
find existing link by normalized URL
   ↓
if found → return existing link
   ↓
generate candidate Base62 code
   ↓
attempt atomic save
   ↓
code collision?
   ├── yes → retry with another code
   └── no  → return stored link
```

Validation, normalization, code generation, persistence logic, and HTTP handling are intentionally separated instead of placing all behavior inside the HTTP handler.

### Package layout

The current internal package structure is:

```text
internal/
├── httpapi/
├── link/
├── shortcode/
└── store/
```

The `link` package contains:

- the `ShortLink` model
- URL validation
- URL normalization
- service/business logic
- the `Store` interface
- domain/service errors

The `store` package contains:

- the `MemoryStore` implementation
- in-memory indexes
- synchronization and locking

The `shortcode` package contains:

- the `Generator` interface
- the random Base62 generator

The `httpapi` package contains:

- HTTP handlers
- request and response DTOs
- route registration
- HTTP error/status mapping
- Swagger annotations

The `cmd/server` package is the application entry point and is responsible for constructing and connecting the concrete dependencies.

### Dependency direction

The shortening service depends on abstractions instead of directly depending on concrete infrastructure.

Conceptually:

```text
             Validator
                 ↑
                 |
ShortenerService → Store interface
                 |
                 ↓
             Generator
```

`MemoryStore` implements the `Store` interface implicitly.

`Base62Generator` implements the generator interface implicitly.

The concrete implementations are constructed in `cmd/server/main.go` and injected into the service.

The HTTP handler receives the already-constructed shortening service instead of constructing dependencies itself.

This keeps dependency wiring at the application entry point.

### Error handling

Expected application errors are represented using sentinel errors where callers need to distinguish their meaning.

Current examples include:

```text
ErrInvalidURL
ErrNotFound
ErrCodeCollision
```

Errors may be wrapped using `%w`.

Higher layers use `errors.Is` to identify a known error category without depending on the error message itself.

For example:

```text
ErrInvalidURL
→ HTTP 400 Bad Request

ErrNotFound
→ HTTP 404 Not Found
```

Unexpected internal failures are logged by the HTTP layer and exposed to clients only as a generic:

```text
500 Internal Server Error
```

This avoids leaking internal implementation details to API clients.

The HTTP layer also uses `errors.As` to recognize `http.MaxBytesError` when the request body exceeds its configured maximum size.

### HTTP API

Part 1 exposes two endpoints.

#### `POST /api/shorten`

The request body is JSON:

```json
{
  "url": "https://example.com"
}
```

A successful request returns:

```text
201 Created
```

with a JSON response containing:

```json
{
  "code": "aB12cD3",
  "short_url": "http://localhost:8080/aB12cD3"
}
```

If the same normalized URL is submitted again, the existing link is returned with the same code while still returning `201 Created`.

Invalid JSON or an invalid URL returns:

```text
400 Bad Request
```

Unexpected service failures return:

```text
500 Internal Server Error
```

The HTTP request body is limited using `http.MaxBytesReader`. Requests exceeding the configured body limit return:

```text
413 Content Too Large
```

#### `GET /{code}`

The short code is read from the URL path.

The handler asks the shortening service to retrieve the corresponding link.

If found, the handler returns:

```text
302 Found
```

with the original long URL as the redirect destination.

If the code does not exist:

```text
404 Not Found
```

is returned.

Unexpected lookup failures return:

```text
500 Internal Server Error
```

### HTTP routing

The project uses Go's standard `net/http` package and Go 1.22+ `ServeMux` routing syntax.

HTTP methods and paths are registered at the routing layer:

```text
POST /api/shorten
GET /{code}
```

The HTTP handlers are responsible for translating HTTP-specific input into service calls and translating service results or errors back into HTTP responses.

Business logic such as normalization, collision handling, and storage synchronization is not implemented in the handlers.

### Request and response DTOs

The HTTP API uses dedicated request and response structs rather than exposing the internal `ShortLink` model directly.

The shortening request contains:

```text
url
```

The shortening response exposes:

```text
code
short_url
```

The internal `CreatedAt` and normalized `LongURL` fields are not exposed by the Part 1 shortening endpoint.

This keeps the HTTP contract separate from the internal domain model.

### Server configuration

Runtime server configuration is provided using command-line flags.

The supported flags are:

```text
-addr
-base
```

`-addr` controls the address on which the HTTP server listens.

The default value is:

```text
:8080
```

`-base` controls the public base URL used when constructing returned short URLs.

The default value is:

```text
http://localhost:8080
```

These values are intentionally separate.

For example, an application could listen internally on:

```text
:8080
```

while returning public URLs based on:

```text
https://example.com
```

This allows the application to work correctly behind a reverse proxy or load balancer.

### Swagger / API documentation

Swagger documentation is generated for the HTTP endpoints.

Swagger-related annotations are kept in the HTTP layer because API documentation is an HTTP concern rather than part of the shortening domain logic.

The Swagger tooling is an additional project dependency and is used only for API documentation and manual API inspection/testing.

### AI assistance

AI was used only as a consultant for specific questions about Go concepts, error handling, concurrency, HTTP handlers, package boundaries, and design tradeoffs. The implementation itself was written and integrated manually.

### Current limitations / TODO

The main Part 1 implementation is complete enough for testing.

Remaining Part 1 work:

- add `httptest` coverage for successful shortening
- test that the same URL returns the same code
- add redirect tests
- add table-driven invalid URL tests
- test unknown codes returning `404`
- test concurrent shortening
- test concurrent shortening of the same URL
- run `go test ./...`
- run `go vet ./...`
- run `go test -race ./...`
- review this document after tests are finalized
