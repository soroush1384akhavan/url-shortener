# Design decisions

## Part 1

### URL normalization

URLs are validated before they are stored.

The current normalization strategy is intentionally conservative. The URL is parsed and normalized without changing path or query semantics.

URLs such as:

- `https://example.com/path`
- `https://example.com/path/`

are currently treated as different URLs.

This avoids modifying URLs in ways that could change their meaning.

Normalization is performed before looking up the URL in the store. This ensures that idempotency is based on the normalized representation rather than the raw request value.

---

### URL validation

A URL must:

- be non-empty
- use the `http` or `https` scheme

The application does not make an HTTP request to the destination URL during validation.

Validation is separated from the shortening service through a `Validator` interface.

The current implementation uses a URL validator, but keeping validation behind an interface allows additional validation rules to be introduced later, such as blocklists or stricter URL policies.

Validation failures are returned as errors instead of boolean-only results so the caller can preserve information about why the operation failed.

---

### Idempotency

The same normalized URL must always return the same short code.

Before generating a new code, the shortening service asks the store whether the normalized URL already exists.

If the URL already exists, the previously stored `ShortLink` is returned and the code generator is not called.

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

The in-memory store maintains a URL-based index so existing links can be found without scanning every stored link.

---

### Link model

A shortened link is represented by a `ShortLink`.

It currently contains:

- `Code`
- `LongURL`
- `CreatedAt`

`CreatedAt` is stored when the link is originally created and uses UTC time.

An existing link is returned from storage instead of constructing a new `ShortLink` for duplicate requests. This preserves metadata such as the original creation time.

---

### Link storage

Part 1 uses an in-memory store.

The store maintains indexes that allow lookup in both directions.

Conceptually:

```text
normalized URL
     ↓
    code
```

and:

```text
code
 ↓
ShortLink
```

The URL index is primarily used for idempotency, while the code index is used for short-code lookup and redirects.

The store implementation is kept separate from the shortening service.

A `Store` interface is defined close to the service that consumes it, while the concrete `MemoryStore` implementation lives in the `store` package.

This allows another storage implementation to be introduced later without tightly coupling the shortening service to the memory store.

---

### Code generation

Short codes are generated only for URLs that do not already exist in the store.

The current generator produces a random 7-character Base62 code.

The alphabet is:

```text
0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz
```

This gives 62 possible values for each character.

With a length of 7, the possible code space is:

```text
62^7
```

which is approximately 3.5 trillion possible codes.

Random values are generated using Go's `crypto/rand` package rather than a basic pseudo-random generator.

Code generation is implemented in the separate `shortcode` package.

A `Generator` interface is used so the generation strategy can later be replaced or mocked during testing.

The code generator is responsible only for generating candidate codes. It does not determine whether a generated code already exists in storage.

---

### Collision handling

Random generation does not guarantee that every generated code is unique.

A generated code therefore must be checked against the store before it is accepted.

An existing mapping must never be silently overwritten by a different URL.

The store currently exposes save behavior intended to reject an already-existing mapping rather than blindly overwrite it.

The final collision strategy is still being refined. The intended behavior is to generate another candidate code when a collision occurs.

Collision handling will remain separate from the code generator because the generator itself does not have access to storage.

---

### Concurrency and locking

The in-memory store may be accessed concurrently by multiple goroutines handling HTTP requests.

Go maps are not safe for concurrent reads and writes, so the memory store is protected using `sync.RWMutex`.

Read operations use:

```go
RLock()
RUnlock()
```

This allows multiple readers to access the store concurrently.

Write operations use:

```go
Lock()
Unlock()
```

This gives write operations exclusive access while the maps are being modified.

The locking is owned by the memory-store implementation rather than by the shortening service. Callers therefore do not need to manage synchronization themselves.

Operations that perform a check followed by a write must also be designed carefully so the combined operation does not introduce a logical race between goroutines.

---

### Shortening flow

The current shortening flow is:

```text
raw URL
   ↓
validate
   ↓
normalize
   ↓
find existing link by URL
   ↓
if found → return existing link
   ↓
generate random Base62 code
   ↓
check/save without overwriting existing data
   ↓
create/store ShortLink
   ↓
return ShortLink
```

Validation, normalization, code generation, and storage are intentionally separated rather than putting all logic in one function or HTTP handler.

---

### Package layout

The current project structure separates the main responsibilities into packages.

```text
internal/
├── link/
├── store/
└── shortcode/
```

The `link` package currently contains:

- `ShortLink`
- URL validation abstractions
- URL normalization
- the shortening service
- the `Store` interface consumed by the service

The `store` package contains:

- the in-memory store implementation
- synchronization for access to in-memory data

The `shortcode` package contains:

- the code-generation interface
- the Base62 random generator

This layout keeps storage and code-generation implementation details outside the shortening service.

---

### Dependency direction

The shortening service depends on abstractions rather than directly depending on `MemoryStore`.

Conceptually:

```text
ShortenerService
      ↓
 Store interface
      ↑
 MemoryStore
```

The concrete store implementation satisfies the interface implicitly, following Go's interface model.

The concrete dependencies are intended to be created and connected in the application entry point rather than inside the shortening service.

---

### Error handling

Operations that can fail return errors.

Validation returns an error when a URL is invalid.

Code generation propagates errors from `crypto/rand`.

Store operations that may fail also return errors.

Errors may be wrapped with `%w` when additional context is useful while preserving the original error for later inspection with `errors.Is`.

The HTTP layer will later be responsible for translating application errors into the required HTTP status codes.

---

### HTTP API

The HTTP layer has not been finalized yet.

Part 1 will expose:

```text
POST /api/shorten
GET  /{code}
```

`POST /api/shorten` will create or return an existing shortened link.

`GET /{code}` will look up the original URL and return a `302 Found` redirect.

The HTTP layer will remain separate from validation, code generation, and store logic.

---

### Server configuration

The server will support:

```text
-addr
```

for the HTTP listen address, and:

```text
-base
```

for constructing the returned short URL.

The default base URL will be:

```text
http://localhost:8080
```

The final values and wiring will be documented after the HTTP server implementation is completed.

---

### Current limitations / TODO

The following Part 1 work is still in progress:

- finalize collision retry behavior
- complete HTTP handlers
- add `-addr` and `-base` flags
- map validation and lookup failures to HTTP status codes
- add tests for shortening and redirects
- add table-driven invalid URL tests
- add unknown-code tests
- add duplicate URL tests
- add concurrent duplicate shortening tests
- verify with `go test -race ./...`