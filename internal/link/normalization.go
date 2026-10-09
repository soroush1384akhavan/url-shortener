package link

import (
	"fmt"
	"net/url"
	"strings"
)

type rule func(u *url.URL)

var rules = []rule{
	lowerSchema,
	lowerHost,
	dropDefaultPort,
	dropDotSegments,
	ensureRootPath,
	dropFragment,
	dropEmptyQuery,
}

func NormalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	for _, r := range rules {
		r(u)
	}
	return u.String(), nil
}

func lowerSchema(u *url.URL) {
	u.Scheme = strings.ToLower(u.Scheme)
}

func lowerHost(u *url.URL) {
	u.Host = strings.ToLower(u.Host)
}

func dropDefaultPort(u *url.URL) {
	port := u.Port()

	if (u.Scheme == "http" && port == "80") ||
		(u.Scheme == "https" && port == "443") {
		u.Host = joinHostPort(u.Hostname())
	}
}

func ensureRootPath(u *url.URL) {
	if u.Path == "" {
		u.Path = "/"
	}
}

func dropDotSegments(u *url.URL) {
	if u.Path == "" {
		return
	}

	cleaned := u.ResolveReference(u)

	u.Path = cleaned.Path
	u.RawPath = cleaned.RawPath
}

func joinHostPort(host string) string {

	if strings.Contains(host, ":") { // for IPv6, ai recommended i didnt know that :)
		return "[" + host + "]"
	}
	return host
}

func dropFragment(u *url.URL) {
	u.Fragment = ""
	u.RawFragment = ""
}

func dropEmptyQuery(u *url.URL) {
	u.ForceQuery = false
}
