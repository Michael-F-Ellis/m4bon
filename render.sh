#!/bin/bash
# render.sh — DSL → MusicXML → SVG → PNG + MIDI playback
# Requires: verovio, rsvg-convert (librsvg)
set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Usage: $0 <dsl-string>"
    echo "   or: $0 -f input.dsl"
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

if [ "$1" = "-f" ]; then
    if [ ! -f "$2" ]; then
        echo "Error: file '$2' not found"
        exit 1
    fi
    TMP=$(mktemp /tmp/m4bon-XXXXXX.mxl)
    MXML=$($M4BON -f "$2")
    echo "$MXML" > "$TMP"
    BASENAME=$(basename "$2" .dsl)
else
    TMP=$(mktemp /tmp/m4bon-XXXXXX.mxl)
    MXML=$($M4BON "$*")
    echo "$MXML" > "$TMP"
    BASENAME="m4bon-render"
fi

# Render PNG
echo "$MXML" | verovio --stdin -f xml -o - -s 120 --adjust-page-height 2>/dev/null \
    | rsvg-convert -b white -o "/tmp/$BASENAME.png"

echo "PNG: /tmp/$BASENAME.png"
open "/tmp/$BASENAME.png"

# Generate and play MIDI (non-blocking)
MIDI="/tmp/$BASENAME.mid"
verovio "$TMP" -f xml -t midi -o "$MIDI" 2>/dev/null
open "$MIDI"

# Clean up
rm -f "$TMP"
