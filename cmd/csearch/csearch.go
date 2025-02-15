// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"

	"codesearch/index"
	"codesearch/regexp"
)

var usageMessage = `usage: csearch [-c] [--allow-files fileregexp] [--block-files fileregexp] [-h] [-i] [-l] [-n] regexp

Csearch behaves like grep over all indexed files, searching for regexp,
an RE2 (nearly PCRE) regular expression.

The -c, -h, -i, -l, and -n flags are as in grep, although note that as per Go's
flag parsing convention, they cannot be combined: the option pair -i -n 
cannot be abbreviated to -in.

The --allow-files flag restricts the search to files whose names match the RE2 regular
expression fileregexp. Multiple flags treated as AND.

The --block-files flag restricts the search to files whose names DOES NOT match the RE2 regular
expression fileregexp. Multiple flags treated as OR.

Csearch relies on the existence of an up-to-date index created ahead of time.
To build or rebuild the index that csearch uses, run:

	cindex path...

where path... is a list of directories or individual files to be included in the index.
If no index exists, this command creates one.  If an index already exists, cindex
overwrites it.  Run cindex -help for more.

Csearch uses the index stored in $CSEARCHINDEX or, if that variable is unset or
empty, $HOME/.csearchindex.
`

func usage() {
	fmt.Fprintf(os.Stderr, usageMessage)
	os.Exit(2)
}

var (
	iFlag              = flag.Bool("i", false, "case-insensitive search")
	onlyListCandidates = flag.Bool("only-list-candidates", false, "list only file candidates for matches")
	verboseFlag        = flag.Bool("verbose", false, "print extra information")
	bruteFlag          = flag.Bool("brute", false, "brute force - search all files in index")
	cpuProfile         = flag.String("cpuprofile", "", "write cpu profile to this file")

	matches bool
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var allowFiles arrayFlags
var blockFiles arrayFlags

func Main() {
	g := regexp.Grep{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	g.AddFlags()
	flag.Var(&allowFiles, "allow-files", "search only files with names matching this regexp")
	flag.Var(&blockFiles, "block-files", "do not search files with names matching this regexp")

	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	if len(args) != 1 {
		usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	pat := "(?m)" + args[0]
	if *iFlag {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		log.Fatal(err)
	}
	g.Regexp = re
	var allowFres = make([]*regexp.Regexp, 0, len(allowFiles))
	if len(allowFiles) > 0 {
		for _, fFlag := range allowFiles {

			var fre, err = regexp.Compile(fFlag)
			if err != nil {
				log.Fatal(err)
			}
			allowFres = append(allowFres, fre)
		}
	}
	var blockFres = make([]*regexp.Regexp, 0, len(blockFiles))
	if len(blockFiles) > 0 {
		for _, fFlag := range blockFiles {

			var fre, err = regexp.Compile(fFlag)
			if err != nil {
				log.Fatal(err)
			}
			blockFres = append(blockFres, fre)
		}
	}
	q := index.RegexpQuery(re.Syntax)
	if *verboseFlag {
		log.Printf("query: %s\n", q)
	}

	ix := index.Open(index.File())
	ix.Verbose = *verboseFlag
	var post []uint32
	if *bruteFlag {
		post = ix.PostingQuery(&index.Query{Op: index.QAll})
	} else {
		post = ix.PostingQuery(q)
	}
	if *verboseFlag {
		log.Printf("post query identified %d possible files\n", len(post))
	}

	if len(allowFres) > 0 || len(blockFres) > 0 {
		fnames := make([]uint32, 0, len(post))

		for _, fileid := range post {
			name := ix.Name(fileid)
			var matches = true
			for _, fre := range allowFres {
				if fre.MatchString(name, true, true) < 0 {
					matches = false
					break
				}
			}
			if !matches {
				continue
			}
			for _, fre := range blockFres {
				if fre.MatchString(name, true, true) >= 0 {
					matches = false
					break
				}
			}
			if matches {
				fnames = append(fnames, fileid)
			}
		}

		if *verboseFlag {
			log.Printf("filename regexp matched %d files\n", len(fnames))
		}
		post = fnames
	}

	for _, fileid := range post {
		name := ix.Name(fileid)
		if *onlyListCandidates {
			fmt.Fprintf(g.Stdout, "%s\n", name)
			g.Limit--
		} else {
			g.File(name)
		}
		if g.Limit == 0 {
			break
		}
	}
}

func main() {
	Main()
	os.Exit(0)
}
