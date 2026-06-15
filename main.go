// m4bon — Beat-Oriented Note Entry to MusicXML converter.
//
// Usage:
//   m4bon "c d e f"                    # DSL from arg, MusicXML to stdout
//   m4bon -f input.dsl                 # DSL from file
//   m4bon -f input.dsl -o output.mxl   # Write to file
//   m4bon -time 6/8 "abc def"          # Specify time signature
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mellis/m4bon/musicxml"
	"github.com/mellis/m4bon/parser"
)

func main() {
	timeSig := flag.String("time", "4/4", "Time signature (e.g. 4/4, 6/8)")
	inputFile := flag.String("f", "", "Read DSL from file instead of argument")
	outputFile := flag.String("o", "", "Write MusicXML to file instead of stdout")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: m4bon [options] [dsl]\n\n")
		fmt.Fprintf(os.Stderr, "Convert m4bon beat-oriented DSL to MusicXML.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  m4bon \"c d e f\"\n")
		fmt.Fprintf(os.Stderr, "  m4bon -time 6/8 \"abc def\"\n")
		fmt.Fprintf(os.Stderr, "  m4bon -f test/cases/basic-notes.dsl -o out.mxl\n")
	}
	flag.Parse()

	// Parse time signature
	var timeNum, timeDen int
	n, err := fmt.Sscanf(*timeSig, "%d/%d", &timeNum, &timeDen)
	if err != nil || n != 2 {
		fmt.Fprintf(os.Stderr, "Invalid time signature: %s (expected e.g. 4/4)\n", *timeSig)
		os.Exit(1)
	}

	// Read DSL
	var dsl string
	if *inputFile != "" {
		data, err := os.ReadFile(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", *inputFile, err)
			os.Exit(1)
		}
		dsl = string(data)
	} else if flag.NArg() > 0 {
		dsl = strings.Join(flag.Args(), " ")
	} else {
		fmt.Fprintln(os.Stderr, "No DSL input provided")
		flag.Usage()
		os.Exit(1)
	}

	dsl = musicxml.SanitizeDSL(dsl)
	if dsl == "" {
		fmt.Fprintln(os.Stderr, "Empty DSL input after sanitization")
		os.Exit(1)
	}

	result := parser.ParseDSL(dsl, timeNum, timeDen)
	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", result.Err)
		os.Exit(1)
	}

	if len(result.Events) == 0 {
		fmt.Fprintln(os.Stderr, "No events produced (empty DSL?)")
		os.Exit(1)
	}

	xml, err := musicxml.Generate(result.Events, timeNum, timeDen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Generation error: %v\n", err)
		os.Exit(1)
	}

	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(xml), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *outputFile, err)
			os.Exit(1)
		}
	} else {
		fmt.Println(xml)
	}
}
