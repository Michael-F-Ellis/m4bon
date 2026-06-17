#!/bin/bash
# imgcat.sh — DSL string → imgcat display (iTerm2 inline PNG) + MIDI playback
# Usage: ./imgcat.sh "c d e f"
#        ./imgcat.sh "KE& M6/8 abc def"
set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Usage: $0 <dsl-string>"
    exit 1
fi

M4BON="./m4bon"

if ! [ -x "$M4BON" ]; then
    # Build from cmd/m4bon/ if not already built
    if [ -d "cmd/m4bon" ]; then
        go build -o "$M4BON" ./cmd/m4bon/
    fi
fi

if ! [ -x "$M4BON" ]; then
    echo "Error: $M4BON not found. Run 'go build -o m4bon ./cmd/m4bon/' first."
    exit 1
fi

TMP=$(mktemp /tmp/m4bon-XXXXXX.mxl)

# Generate MusicXML and save to temp file
MXML=$("$M4BON" "$*")
echo "$MXML" > "$TMP"

# Render PNG inline via Verovio + rsvg-convert + imgcat
echo "$MXML" | verovio --stdin -f xml -o - -s 120 --adjust-page-height 2>/dev/null \
    | rsvg-convert -b white \
    | imgcat

# Generate and play MIDI (non-blocking)
MIDI="${TMP%.mxl}.mid"
verovio "$TMP" -f xml -t midi -o "$MIDI" 2>/dev/null
open "$MIDI"

# Clean up temp files after a short delay (MIDI needs the file to exist briefly)
(sleep 5; rm -f "$TMP" "$MIDI") &
