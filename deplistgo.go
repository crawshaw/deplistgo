// Deplistgo generates a list of files relevant to building a package.
//
// The output is in the deplist format used by ninja. It is a subset
// of the Makefile format:
//
//	bar.o: bar.c foo.h
//	bar.c:
//	foo.h:
package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type stringsFlag []string

func (v *stringsFlag) String() string { return "<stringsFlag>" }

func (v *stringsFlag) Set(s string) error {
	*v = strings.Split(s, " ")
	if *v == nil {
		*v = []string{}
	}
	return nil
}

var ctx = build.Default

func main() {
	flag.Var((*stringsFlag)(&ctx.BuildTags), "tags", "")
	flag.Parse()

	roots := flag.Args()
	if len(roots) == 0 {
		fmt.Fprintf(os.Stderr, "usage: deplistgo [packages]\n")
		os.Exit(2)
	}

	var mu sync.Mutex
	outputs := make(map[string]string)
	deps := make(map[string][]string)

	fdlimit := make(chan struct{}, 128)
	var wg sync.WaitGroup
	var scan func(path string)
	scan = func(path string) {
		defer wg.Done()

		mu.Lock()
		_, done := deps[path]
		if !done {
			deps[path] = nil // sentinel
		}
		mu.Unlock()

		if done {
			return
		}

		fdlimit <- struct{}{}
		defer func() { <-fdlimit }()

		pkg, err := ctx.Import(path, "", 0)
		if err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		var files []string
		srcdir := pkg.Root + "/src/"
		files = appendAndPrefix(files, srcdir, pkg.GoFiles)
		files = appendAndPrefix(files, srcdir, pkg.CgoFiles)
		files = appendAndPrefix(files, srcdir, pkg.CFiles)
		files = appendAndPrefix(files, srcdir, pkg.CXXFiles)
		files = appendAndPrefix(files, srcdir, pkg.HFiles)
		files = appendAndPrefix(files, srcdir, pkg.SFiles)
		files = appendAndPrefix(files, srcdir, pkg.SwigFiles)
		files = appendAndPrefix(files, srcdir, pkg.SwigCXXFiles)

		outdir := pkg.Root
		var output string
		if pkg.Name == "main" {
			if ctx.GOOS == runtime.GOOS && ctx.GOARCH == runtime.GOARCH {
				output = outdir + "/bin/" + path
			} else {
				output = outdir + "/bin/" + ctx.GOOS + "_" + ctx.GOARCH + "/" + path
			}
		} else {
			output = pkg.PkgObj
		}

		mu.Lock()
		outputs[path] = output
		deps[path] = files
		mu.Unlock()

		for _, imp := range pkg.Imports {
			wg.Add(1)
			go scan(imp)
		}
	}

	for _, root := range roots {
		wg.Add(1)
		go scan(root)
	}
	wg.Wait()

	var paths []string
	for path := range deps {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		fmt.Printf("%s:", outputs[path])
		for _, dep := range deps[path] {
			fmt.Printf(" %s", dep)
		}
		fmt.Print("\n")
	}
}

func appendAndPrefix(slice []string, prefix string, src []string) []string {
	for _, s := range src {
		slice = append(slice, prefix+s)
	}
	return slice
}
