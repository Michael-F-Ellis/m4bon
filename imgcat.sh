#!/bin/bash
# imgcat.sh — DSL string → imgcat display (iTerm2 inline PNG)
# Usage: ./imgcat.sh "c d e f"
#        ./imgcat.sh "KE& M6/8 abc def"
set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Usage: $0 <dsl-string>"
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

"$M4BON" "$*" | verovio --stdin -f xml -o - -s 120 --adjust-page-height 2>/dev/null \
    | rsvg-convert -b white \
    | imgcat
