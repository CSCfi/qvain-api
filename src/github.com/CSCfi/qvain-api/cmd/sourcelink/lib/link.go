// Package sourcelink tries to convert a repository url and hash to a link for the current source tree.
//
// If it fails to parse and convert the link and the url is to a http(s) location, it returns the original url;
// if the original url was to another protocol scheme, it returns the empty string.
package sourcelink

import (
	neturl "net/url"
	"regexp"
	"strings"
)

// httpPrefix is the prefix added to generated http(s) links.
const httpPrefix = "https"

// repoFunc is a function that takes url, hash and branch string arguments and returns a new url.
type repoFunc func(url, hash, branch string) string

// For these repo types we know how to make a direct link to the source tree.
var treeFor = map[string]repoFunc{
	"github": func(url, hash, branch string) string {
		return url + "/tree/" + neturl.PathEscape(hash)
	},
	"cgit": func(url, hash, branch string) string {
		return url + "/tree/?h=" + neturl.QueryEscape(branch) + "&id=" + neturl.QueryEscape(hash)
	},
	"bitbucket": func(url, hash, branch string) string {
		return url + "/" + neturl.PathEscape(hash) + "/?at=" + neturl.QueryEscape(branch)
	},
	"gitlab": func(url, hash, branch string) string {
		return url + "/tree/" + neturl.PathEscape(hash)
	},
	"gitweb": func(url, hash, branch string) string {
		return url + "/tree/?h=" + neturl.QueryEscape(branch) + "id=" + neturl.QueryEscape(hash)
	},
}

// parseScpUrl parses a git scp url of the form [user@]<host.name:>[path] and returns user, host and path. If the host field is empty the regexp failed.
func parseScpUrl(url string) (string, string, string) {
	// not bulletproof but safe; note we assume path part is already escaped, otherwise we'll likely end up double-escaping
	re := regexp.MustCompile(`^(?:([a-zA-Z0-9_.-]+@))?([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9.]*[a-zA-Z0-9]):(?:(.*))?$`)
	m := re.FindStringSubmatch(url)
	if len(m) > 0 {
		return m[1], m[2], m[3]
	}
	return "", "", ""
}

// isHttpProtocol checks if the protocol of a given url is http or https
func isHttpProtocol(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// getUrlScheme returns the scheme (protocol) of a URL. If it fails to find one, it returns an empty string.
// NOTE: The length of the scheme string is capped at 24 characters.
func getUrlScheme(url string) string {
	return fastScheme(url, "://")
}

// getUriScheme returns the scheme (protocol) of a URI. If it fails to find one, it returns an empty string.
// NOTE: The length of the scheme string is capped at 24 characters.
func getUriScheme(url string) string {
	return fastScheme(url, ":")
}

// fastScheme returns a prefix of valid "URL/URI scheme" characters up to the given separator or empty string if it can't parse the given URL/URI.
func fastScheme(u, sep string) string {
	if u == "" {
		return ""
	}

	var end int
	for i, c := range u {
		// [a-z][a-z+.-]*
		if !(c >= 'a' && c <= 'z') {
			if !(i > 0 && (c == '+' || c == '.' || c == '-')) {
				break
			}
		}

		end++

		// sanity limit
		if i >= 23 {
			break
		}
	}

	if len(u) >= end+len(sep) && u[end:end+len(sep)] == sep {
		return u[0:end]
	}
	return ""
}

func tertiary(cond bool, t string, f string) string {
	if cond {
		return t
	}
	return f
}

// MakeSourceLink tries to modify a repository's home link to point to the file tree for the given commit.
// On failure, it returns the given url for http urls and an empty string for ssh urls.
func MakeSourceLink(url, hash, branch string) string {
	var host string

	// no needless work
	if url == "" {
		return ""
	}

	// strip trailing slash; we know len(url) > 0
	if url[len(url)-1] == '/' {
		url = url[0 : len(url)-1]
	}

	// net/url can't parse URLs without scheme
	scheme := getUrlScheme(url)
	isHttp := scheme == "http" || scheme == "https"

	// either scheme:// or scp form
	if scheme != "" {
		parsed, err := neturl.Parse(url)
		if err != nil {
			return tertiary(isHttp, url, "")
		}

		// change the scheme to http for non-http urls
		if !isHttp {
			parsed.Scheme = httpPrefix
			url = parsed.String()
		}
		host = parsed.Hostname()
	} else if strings.IndexByte(url, ':') > 0 {
		var path string
		_, host, path = parseScpUrl(url)

		if host != "" {
			url = httpPrefix + "://" + host + "/" + strings.TrimSuffix(path, ".git")
		}
	}

	// failed to parse
	if host == "" {
		return tertiary(isHttp, url, "")
	}

	var fn repoFunc
	switch host {
	case "github.com":
		fn = treeFor["github"]
	case "git.zx2c4.com":
		fn = treeFor["cgit"]
	case "bitbucket.org":
		fn = treeFor["bitbucket"]
	}

	// call appropriate function for repo host type
	if fn != nil {
		return fn(url, hash, branch)
	}

	// don't return anything if we failed to convert non-http urls
	return tertiary(isHttp, url, "")
}
