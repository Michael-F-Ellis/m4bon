# m4bon

**Beat-Oriented Note Entry → MusicXML Converter**

A CLI tool that converts m4bon DSL (a compact rhythmic notation) into standard MusicXML files readable by MuseScore, Finale, Dorico, and other notation software.

## Quick Start

```bash
go build -o m4bon .
./m4bon "c d e f" > notes.mxl
# Open notes.mxl in your notation software
```

## DSL in 30 seconds

Whitespace separates beats. Characters within a beat subdivide it equally.

| Input | Meaning in 4/4 |
|-------|----------------|
| `a b` | Quarter C + quarter D |
| `ab` | Two eighth notes (C + D) |
| `abc` | Triplet eighths (C + D + E) |
| `a--b` | Dotted eighth C + sixteenth D |
| `(ace)f` | A-minor chord eighth + F eighth |
| `a - -b c` | C tied over 2.5 beats + D eighth + E quarter |

Full reference in [AGENTS.md](AGENTS.md)

## Usage

```bash
m4bon "c d e f"                    # MusicXML to stdout
m4bon -time 6/8 "abc def"          # Specify time signature
m4bon -f input.dsl -o output.mxl   # File in, file out
```

## Why MusicXML?

MusicXML is the interchange format used by all major notation software (MuseScore, Finale, Dorico). It has native support for ties, tuplets, beaming, and accidentals — unlike MIDI — and is plain text (diff-able, grep-able, version-controllable).

The project started as a MuseScore 4 QML plugin but switched to Go + MusicXML when the plugin API proved too constrained (no headless mode, no debugger, 8-25s edit-test cycles). Go's `encoding/xml` and sub-second test cycles via `go test` made MusicXML output the right choice.

## Development

```bash
go test ./...              # 0.4s — full regression suite
go build -o m4bon .        # 0.5s — standalone binary
./m4bon "c d e f"          # instant — pipe into any editor
```

## License

MIT
