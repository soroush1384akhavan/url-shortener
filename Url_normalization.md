# URL Normalization Guide

Different URLs that point to the same resource, and how to normalize them.

Rules are split into two groups:

- **Safe**: guaranteed equivalent by the URL standards (RFC 3986 / WHATWG URL). Apply to every URL.
- **Heuristic**: usually equivalent, but depends on the site. Apply only if you know your target sites.

---

## 1. Safe normalizations (always the same)

| Variation | Example A | Example B | Normalized |
|---|---|---|---|
| Scheme case | `HTTP://example.com/` | `http://example.com/` | lowercase scheme |
| Host case | `http://EXAMPLE.Com/` | `http://example.com/` | lowercase host |
| Default port | `http://example.com:80/` | `http://example.com/` | drop `:80` (http) / `:443` (https) |
| Empty path | `http://example.com` | `http://example.com/` | add `/` |
| Dot segments | `/a/./b/../c` | `/a/c` | resolve `.` and `..` |
| Percent-encoding case | `/caf%c3%a9` | `/caf%C3%A9` | uppercase hex digits |
| Needlessly encoded characters | `/%7Euser` | `/~user` | decode unreserved chars (`A-Z a-z 0-9 - . _ ~`) |
| Trailing dot in host | `example.com.` | `example.com` | strip the dot |
| Unicode domain | `münchen.de` | `xn--mnchen-3ya.de` | convert to punycode |
| Fragment | `/page#section2` | `/page` | drop the fragment (never sent to the server) |
| Empty query/fragment | `/page?` | `/page` | drop the lone `?` or `#` |
| IP address formats | `http://2130706433/` | `http://127.0.0.1/` | convert to dotted decimal (also hex/octal forms) |
| Empty userinfo | `http://@example.com/` | `http://example.com/` | drop empty userinfo |

---

## 3. Practical tips

- **Always apply the safe rules.** Apply heuristic rules only when you know the target sites; on some sites `/Page` and `/page` are different resources.
- **Sort query parameters, but keep meaningful ones.** Don't remove params that change content (`?page=2`, `?id=5`).
- **Strip tracking params using a list:** `utm_*`, `fbclid`, `gclid`, `mc_eid`, `ref`, etc.
- **For ground truth,** follow redirects and read the page's `<link rel="canonical">` tag, which is what the site itself says is the "real" URL.
- **Decide on a trailing-slash policy** and apply it consistently (keep or strip), since servers differ.
- **Store both** the original and the normalized URL, so you never lose information.

---


### Known limitations of the example

- `posixpath.normpath` collapses duplicate slashes (`/a//b` becomes `/a/b`), which is a heuristic rule, not a safe one.
- Decoding the whole path with `unquote` then re-quoting can change meaning for encoded reserved characters such as `%2F` (an encoded slash). A stricter implementation should only decode unreserved characters.
- `parse_qsl` drops empty values by default (`?a=&b=2` loses `a`). Pass `keep_blank_values=True` if you need to preserve them.
- Sorting parameters changes the order of repeated keys' relationships only when you sort by key alone; sorting `(key, value)` pairs as above is stable and deterministic.

---

## 5. Test cases

All of these should normalize to `http://example.com/a/c`:

```
HTTP://EXAMPLE.com/a/c
http://example.com:80/a/c
http://example.com/a/./b/../c
http://example.com./a/c
http://example.com/a/c#section
http://example.com/a/c?
```

---

## 6. References

- RFC 3986, Section 6: Normalization and Comparison
- WHATWG URL Standard: https://url.spec.whatwg.org/
- RFC 6596: The Canonical Link Relation