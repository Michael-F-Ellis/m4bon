#!/bin/bash
# render.sh — DSL → MusicXML → SVG → PNG
# Requires: verovio, rsvg-convert (librsvg)
set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Usage: $0 <dsl-string>"
    echo "   or: $0 -f input.dsl"
    exit 1
fi

M4BON="./m4bon"

if ! [ -x "$M4BON" ]; then
    if [ -f "$M4BON" ]; then
        chmod +x "$M4BON"
    else
        echo "Error: $M4BON not found. Run 'go build -o m4bon .' first."
        exit 1
    fi
fi

if [ "$1" = "-f" ]; then
    if [ ! -f "$2" ]; then
        echo "Error: file '$2' not found"
        exit 1
    fi
    XML=$($M4BON -f "$2")
else
    XML=$($M4BON "$*")
fi

echo "$XML" | verovio --stdin -f xml -o - -s 120 --adjust-page-height 2>/dev/null \
    | rsvg-convert -b white -o /tmp/m4bon-render.png

echo "Output: /tmp/m4bon-render.png"
open /tmp/m4bon-render.png
