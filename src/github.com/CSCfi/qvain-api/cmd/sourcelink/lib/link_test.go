package sourcelink

import (
	"testing"
)

type Params struct {
	url    string
	hash   string
	branch string
}

func TestMakeSourceLink(t *testing.T) {
	var tests = []struct {
		name   string
		params []string // url, hash, branch
		exp    string
	}{
		{name: "empty", params: []string{"", "abcdef", "master"}, exp: ""},
		{name: "github (http)", params: []string{"https://github.com/user/repo", "abcdef", "master"}, exp: "https://github.com/user/repo/tree/abcdef"},
		{name: "github (ssh)", params: []string{"git@github.com:user/repo.git", "abcdef", "master"}, exp: "https://github.com/user/repo/tree/abcdef"},
		{name: "cgit (http) [TrailingSlash QueryEscape]", params: []string{"https://git.zx2c4.com/WireGuard/", "abcdef", "jd/no-inline"}, exp: "https://git.zx2c4.com/WireGuard/tree/?h=jd%2Fno-inline&id=abcdef"},
		{name: "unknown repo", params: []string{"git@example.com:user/repo", "abcdef", "master"}, exp: ""},
		{name: "cgit (git)", params: []string{"git://git.zx2c4.com/WireGuard", "abcdef", "master"}, exp: "https://git.zx2c4.com/WireGuard/tree/?h=master&id=abcdef"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			url := MakeSourceLink(test.params[0], test.params[1], test.params[2])
			if url != test.exp {
				t.Errorf("fail for test %s: expected %s, got %s", test.name, test.exp, url)
			}
		})
	}
}
