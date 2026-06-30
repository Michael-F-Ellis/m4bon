# m4bon Examples

Full-length songs transcribed into the m4bon beat-oriented DSL.

Each `.dsl` file is a complete, playable transcription. Open any file in the
[m4bon web app](https://mellis.github.io/m4bon/) or compile to MusicXML:

```bash
m4bon -f examples/twinkle-twinkle.dsl -o twinkle.mxl
```

## Available Examples

| File | Piece | Composer | Key | Meter |
|------|-------|----------|-----|-------|
| `three-blind-mice.dsl` | Three Blind Mice (Round) | Traditional | C major | 4/4 |
| `twinkle-twinkle.dsl` | Twinkle Twinkle Little Star | Traditional | C major | 4/4 |
| `ode-to-joy.dsl` | Ode to Joy (Theme) | Beethoven | C major | 4/4 |
| `minuet-in-g.dsl` | Minuet in G (BWV Anh. 114) | Petzold/Bach | G major | 3/4 |
| `happy-birthday.dsl` | Happy Birthday | Traditional | F major | 3/4 |
| `frere-jacques.dsl` | Frère Jacques (Round) | Traditional | C major | 4/4 |
| `jingle-bells.dsl` | Jingle Bells | Pierpont | C major | 4/4 |
| `greensleeves.dsl` | Greensleeves | Traditional | E minor | 6/8 |

## DSL Quick Reference

Whitespace separates beats. Characters within a beat subdivide it equally.

| Input | Meaning in 4/4 |
|-------|----------------|
| `a b` | Quarter C + quarter D |
| `ab` | Two eighth notes (C + D) |
| `abc` | Triplet eighths (C + D + E) |
| `a--b` | Dotted eighth C + sixteenth D |
| `(ace)f` | A-minor chord eighth + F eighth |
| `a - -b c` | C tied over 2.5 beats + D eighth + E quarter |

Full DSL reference: [AGENTS.md](../AGENTS.md)
