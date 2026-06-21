// m4bon — Beat-Oriented Note Entry to MusicXML converter.
//
// Usage:
//   m4bon "c d e f"                    # DSL from arg, MusicXML to stdout
//   m4bon -f input.dsl                 # DSL from file
//   m4bon -f input.dsl -o output.mxl   # Write to file
//   m4bon -time 6/8 "abc def"          # Specify time signature
//   m4bon -tui                         # Launch interactive TUI
//   m4bon -tui -f input.dsl            # Launch TUI with file loaded
//   m4bon -tui -f input.dsl -bpm 96    # Launch TUI with custom BPM
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mellis/m4bon"
	"github.com/mellis/m4bon/cmd/m4bon/tui"
	"github.com/mellis/m4bon/musicxml"
	"github.com/mellis/m4bon/parser"
	"github.com/mellis/m4bon/render"
)

func main() {
	inputFile := flag.String("f", "", "Read DSL from file instead of argument")
	outputFile := flag.String("o", "", "Write MusicXML to file instead of stdout")
	renderFlag := flag.Bool("render", false, "Output colorized text format instead of MusicXML")
	asciiLeaps := flag.Bool("ascii-leaps", false, "Use ANSI escapes for leap indicators instead of Unicode diacritics")
	tuiFlag := flag.Bool("tui", false, "Launch the interactive TUI performance/learning tool")
	bpmFlag := flag.Float64("bpm", 120, "Tempo in beats per minute (for TUI mode)")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: m4bon [options] [dsl]\n\n")
		fmt.Fprintf(os.Stderr, "m4bon v%s — beat-oriented note entry to MusicXML.\n\n", m4bon.Version)
		fmt.Fprintf(os.Stderr, "Time and key signatures are specified in the DSL:\n")
		fmt.Fprintf(os.Stderr, "  M4/4 c d e f     (meter, default 4/4)\n")
		fmt.Fprintf(os.Stderr, "  KE& M6/8 abc def (key sig + meter)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  m4bon \"c d e f\"\n")
		fmt.Fprintf(os.Stderr, "  m4bon \"KE& M6/8 abc def\"\n")
		fmt.Fprintf(os.Stderr, "  m4bon -f test/cases/basic-notes.dsl -o out.mxl\n")
		fmt.Fprintf(os.Stderr, "  m4bon -render \"M4/4 c d e f\"\n")
		fmt.Fprintf(os.Stderr, "  m4bon -tui\n")
		fmt.Fprintf(os.Stderr, "  m4bon -tui -f score.dsl -bpm 96\n")
	}
	flag.Parse()

	// Version flag
	if *versionFlag {
		fmt.Printf("m4bon v%s\n", m4bon.Version)
		return
	}

	// Read DSL
	var dsl string
	var dslLabel string
	if *inputFile != "" {
		data, err := os.ReadFile(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", *inputFile, err)
			os.Exit(1)
		}
		dsl = string(data)
		dslLabel = *inputFile
	} else if flag.NArg() > 0 {
		dsl = strings.Join(flag.Args(), " ")
		dslLabel = "arg"
	} else if !*tuiFlag {
		fmt.Fprintln(os.Stderr, "No DSL input provided")
		flag.Usage()
		os.Exit(1)
	}

	// TUI mode
	if *tuiFlag {
		dsl = parser.SanitizeDSL(dsl)
		err := tui.Run(dsl, dslLabel, *bpmFlag, *asciiLeaps)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	dsl = parser.SanitizeDSL(dsl)
	if dsl == "" {
		fmt.Fprintln(os.Stderr, "Empty DSL input after sanitization")
		os.Exit(1)
	}

	result := parser.ParseDSL(dsl)
	if result.Err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %v\n", result.Err)
		os.Exit(1)
	}

	if len(result.Measures) == 0 {
		fmt.Fprintln(os.Stderr, "No events produced (empty DSL?)")
		os.Exit(1)
	}

	if *renderFlag {
		out := render.Render(result.Measures, *asciiLeaps, true)
		if *outputFile != "" {
			if err := os.WriteFile(*outputFile, []byte(out), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *outputFile, err)
				os.Exit(1)
			}
		} else {
			fmt.Print(out)
		}
		return
	}

	xml, err := musicxml.Generate(result.Measures, result.Key.Fifths)
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
