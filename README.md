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

## Why MusicXML instead of a MuseScore plugin?

The original version was a MuseScore 4 QML plugin (`m4bon.qml`). Plugin development on MS4.7 proved extremely constrained:

- No headless/CLI invocation — every change required a full MuseScore restart
- Ties couldn't be created via the plugin API (8 approaches attempted, all failed)
- `XMLHttpRequest` sandboxed — couldn't read test files from within the plugin
- "Reload Plugins" button broken — changes required app restart
- No debugger — only `console.log` to terminal

Switching to MusicXML eliminated every constraint: sub-second test cycle, native tie support, diff-able output, zero runtime dependencies.

## Development

```bash
go test ./...              # 0.4s — full regression suite
go build -o m4bon .        # 0.5s — standalone binary
./m4bon "c d e f"          # instant — pipe into any editor
```

## License

MIT
