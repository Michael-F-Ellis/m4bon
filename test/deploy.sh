#!/bin/bash
# test/deploy.sh — Deploy plugin + test cases to MuseScore, then open
#
# Usage:
#   ./test/deploy.sh                          # Deploy and open MuseScore
#   ./test/deploy.sh test/cases/triplet.dsl   # Deploy with a specific test pre-loaded
#
# After deploy: Plugins → Composing/arranging tools → m4bon
# Then click "Load" to load a test case from the dropdown, or "Send" to run.
#
# For console.log output, launch MuseScore from terminal first:
#   /Applications/MuseScore\ 4.app/Contents/MacOS/mscore &
# Then use this script to deploy (it won't re-launch if MuseScore is running).
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PLUGIN_DIR="$HOME/Documents/MuseScore4/Plugins/m4bon"
FIXTURE="$REPO_DIR/test/fixtures/empty.mscz"

# Deploy plugin QML
mkdir -p "$PLUGIN_DIR"
cp "$REPO_DIR/m4bon.qml" "$PLUGIN_DIR/"

# Deploy test cases to cases/ subdirectory (readable via Qt.resolvedUrl)
mkdir -p "$PLUGIN_DIR/cases"
cp "$REPO_DIR"/test/cases/*.dsl "$PLUGIN_DIR/cases/" 2>/dev/null || true

echo "Deployed m4bon.qml + $(ls "$PLUGIN_DIR/cases/"*.dsl 2>/dev/null | wc -l | tr -d ' ') test cases"

# Pre-load a specific test case if provided
if [ $# -ge 1 ] && [ -f "$1" ]; then
    grep -v '^#' "$1" | tr -d '\n' > /tmp/m4bon-dsl.txt
    echo "Pre-loaded: $1"
fi

# Launch MuseScore with the test fixture if not already running
if ! pgrep -q "mscore"; then
    echo "Launching MuseScore with test fixture..."
    if [ -f "$FIXTURE" ]; then
        open -a "MuseScore 4" "$FIXTURE"
    else
        echo "No test fixture at $FIXTURE — create one first"
        echo "  MuseScore → New → 4/4, 1 measure, Piano → Save as $FIXTURE"
    fi
else
    echo "MuseScore already running — click Plugins → Composing/arranging → m4bon"
fi
