# Ties not created by plugin

**Status:** Deferred by user decision  
**Date:** 2026-06-14  
**Plugin version:** 0.5.0+

## Problem

Non-standard note durations produced by sustain chains (e.g. `a - -b c` yields `a` = 5/8 of a whole note = 2.5 beats in 4/4) must be split into standard note values for MuseScore to render them. The split produces two notes (e.g. half A + eighth A) that should be tied together per engraving rules (TIES-VS-DOTS.md). The plugin creates correct durations and positions but the tie between the split fragments is never created.

## Engraving context

Per TIES-VS-DOTS.md, the half + eighth split is correct because the duration crosses the "invisible barline" at beat 3 of 4/4. The two fragments SHOULD be tied. Even durations that are standard note values may need splitting based on where they fall in the measure (e.g. a dotted half starting on beat 2 must be quarter tied to half).

## Attempted fixes (all failed)

| # | Approach | Result |
|---|---|---|
| 1 | `note.tieFor = otherNote` | Read-only property in MS4 API |
| 2 | `newElement(Element.TIE)` + `note.add(tie)` | Crashes MS4 (community report) |
| 3 | `curScore.runCommand("tie")` after addNote | Silent no-op |
| 4 | `cmd("tie")` with `curScore.selection.select(chord)` | Silent no-op |
| 5 | `cmd("tie")` after `setDuration` (Method 2 cursor workflow) | Omitted the second note entirely |
| 6 | `curScore.selection.selectRange()` + `cmd("tie")` | Silent no-op |
| 7 | Navigate cursor by tick to find element, select Note | No output, no error |
| 8 | Track Note ref from `addNote()`, select Note directly + `cmd("tie")` | Silent no-op |

The working pattern reported in community plugins — `curScore.selection.select(note)` on an existing Note object followed by `cmd("tie")` — does not work for notes created via `cursor.addNote()` in our plugin.

## Current behavior

- `splitNonStandardDurations()` correctly splits non-standard durations into standard note values
- The split fragments have correct positions and durations
- No tie is created between the fragments
- The user can manually select both notes in MuseScore UI and click the T (tie) button

## User decision

> "I'm making a decision to defer the problem for now. The MuseScore UI makes it easy to tie existing notes by selecting them and clicking the tie button. For now, we'll settle for correct durations and leave tie decisions to the user."

## Future considerations

The `splitNonStandardDurations()` function currently uses a naive greedy split (largest standard values first). This does not respect engraving rules from TIES-VS-DOTS.md. A future version should:

1. Track the cursor's current beat position within the measure
2. Split durations at barline, midpoint (beat 3 of 4/4), and beat boundaries
3. Once ties work, the visual result will follow proper engraving conventions

The tie creation mechanism is deferred until we discover a reliable API or workaround for MuseScore 4.7.2.
