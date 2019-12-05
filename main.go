// Command sourcelink tries to return a link to the source tree for a given hash and base repository path.
//
// It will try to convert git://, ssh:// and scp-style user@host.name:path.git links to http urls.
// If it fails to convert the url, it will print the given repo url and return with a non-zero exit code.
package main

import (
	"fmt"
	"os"

	sourcelink "github.com/wvh/sourcelink/lib"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 4 {
		fmt.Println("usage:", os.Args[0], "<repo url> <hash> [branch]")
		if len(os.Args) < 2 {
			fmt.Println("")
			fmt.Println("This program tries to return a link to the source tree for a given hash and base repository path.")
			fmt.Println("It will try to convert git://, ssh:// and scp-style user@host.name:path.git links to http urls.")
			fmt.Println("If it fails to convert the url, it will print the given repo url and return with a non-zero exit code.")
			fmt.Println("")
			fmt.Println("Examples:")
			fmt.Println("")
			fmt.Println("HTTP upstream")
			fmt.Println("\t$", os.Args[0], "https://github.com/user/repo abcdef")
			fmt.Println("\thttps://github.com/user/repo/tree/abcdef")
			fmt.Println("")
			fmt.Println("SCP upstream `git@github.com:user/repo.git` taken from git config")
			fmt.Println("\t$", os.Args[0], "$(git ls-remote --get-url 2>/dev/null) $(shell git rev-parse --short HEAD 2>/dev/null)")
			fmt.Println("\thttps://github.com/user/repo/tree/abcdef")
			fmt.Println("")
			fmt.Println("Cgit upstream")
			fmt.Println("\t$", os.Args[0], "git://git.zx2c4.com/WireGuard/ 07a03cbc8d186f985bcccede99fc3547f23868d8 jd/no-inline")
			fmt.Println("\thttps://git.zx2c4.com/WireGuard/tree/?h=jd%2Fno-inline&id=07a03cbc8d186f985bcccede99fc3547f23868d8")
			fmt.Println("")
		}
		os.Exit(1)
	}

	var repo, hash, branch string
	repo = os.Args[1]

	if len(os.Args) > 2 {
		hash = os.Args[2]
	}

	if len(os.Args) > 3 {
		branch = os.Args[3]
	}

	/*
		fmt.Println("repo:", repo)
		fmt.Println("hash:", hash)
		fmt.Println("branch:", branch)
	*/

	link := sourcelink.MakeSourceLink(repo, hash, branch)
	if link != "" {
		fmt.Println(link)
		os.Exit(0)
	}
	fmt.Println(repo)
	os.Exit(1)
}
