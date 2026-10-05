package link

import (
	"net"
	"net/url"
	"path"
	"strings"
)

type rule func(u *url.URL) error

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
		return "", err
	}
	for _, r := range rules {
		if err := r(u); err != nil {
			return "", err
		}
	}
	return u.String(), nil
}

func lowerSchema(u *url.URL) error {
	u.Scheme = strings.ToLower(u.Scheme)
	return nil
}

func lowerHost(u *url.URL) error {
	u.Host = strings.ToLower(u.Host)
	return nil
}

func dropDefaultPort(u *url.URL) error {
	port := u.Port()

	if (u.Scheme == "http" && port == "80") ||
		(u.Scheme == "https" && port == "443") {
		u.Host = joinHostPort(u.Hostname(), "")
	}
	return nil
}

func ensureRootPath(u *url.URL) error {
	if u.Path == "" {
		u.Path = "/"
	}
	return nil
}

func dropDotSegments(u *url.URL) error {
	if u.Path == "" {
		return nil
	}

	u.Path = path.Clean(u.Path)
	u.RawPath = ""
	return nil
}

func joinHostPort(host, port string) string {
	if port == "" {
		if strings.Contains(host, ":") { // for IPv6, ai recommended i didnt know that :)
			return "[" + host + "]"
		}
		return host
	}
	return net.JoinHostPort(host, port)
}

func dropFragment(u *url.URL) error {
	u.Fragment = ""
	u.RawFragment = ""
	return nil
}

func dropEmptyQuery(u *url.URL) error {
	u.ForceQuery = false
	return nil
}
