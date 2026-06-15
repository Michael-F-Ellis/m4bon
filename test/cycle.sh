#!/bin/bash
# test/cycle.sh — Run m4bon test cases by automating MuseScore via AppleScript
#
# Prerequisites:
#   1. Enable m4bon-cli in Plugin Manager at least once:
#      MuseScore → Home → Plugins → find "m4bon-cli" → Enable
#
#   2. Create a minimal test score:
#      MuseScore → New → 4/4, 1 measure, Piano → Save as test/fixtures/empty.mscz
#
#   3. When MuseScore opens during a test, leave it alone — AppleScript
#      will drive it automatically.
#
# Usage:
#   ./test/cycle.sh                              # Run all test cases
#   ./test/cycle.sh test/cases/basic-notes.dsl   # Run specific case(s)
#
set -euo pipefail

MSCORE="/Applications/MuseScore 4.app/Contents/MacOS/mscore"
MSCORE_BIN="/Applications/MuseScore 4.app/Contents/MacOS/musescore"
PLUGIN_DIR="$HOME/Documents/MuseScore4/Plugins"
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DSL_FILE="/tmp/m4bon-dsl.txt"
FIXTURE="$REPO_DIR/test/fixtures/empty.mscz"
RESULT="/tmp/m4bon-result.mscz"

# Colors
PASS="\033[32mPASS\033[0m"
FAIL="\033[31mFAIL\033[0m"
INFO="\033[36m"
RESET="\033[0m"

# Check prerequisites
if [ ! -f "$FIXTURE" ]; then
    echo "No test fixture found at $FIXTURE"
    echo "Create one: MuseScore → New → 4/4, 1 measure → Save as that path"
    exit 1
fi

# Deploy CLI plugin
mkdir -p "$PLUGIN_DIR/m4bon-cli"
cp "$REPO_DIR/m4bon-cli.qml" "$PLUGIN_DIR/m4bon-cli/"
echo -e "${INFO}Plugin deployed to $PLUGIN_DIR/m4bon-cli/${RESET}"

# Determine which test cases to run
if [ $# -gt 0 ]; then
    CASES=("$@")
else
    CASES=( "$REPO_DIR/test/cases/"*.dsl )
fi

TOTAL=0
PASSED=0

for dsl in "${CASES[@]}"; do
    NAME=$(basename "$dsl" .dsl)
    TOTAL=$((TOTAL + 1))
    echo ""
    echo "=== $NAME ==="

    # Write DSL (strip comment lines) to temp file
    grep -v '^#' "$dsl" | tr -d '\n' > "$DSL_FILE"

    if [ ! -s "$DSL_FILE" ]; then
        echo -e "  $FAIL (empty DSL)"
        continue
    fi
    echo "  DSL: $(cat "$DSL_FILE")"

    # Kill any running MuseScore
    pkill -9 -f "MuseScore 4" 2>/dev/null || true
    sleep 1

    # Launch MuseScore with the test score, capturing console.log output
    "$MSCORE_BIN" "$FIXTURE" &
    MS_PID=$!
    sleep 3

    # Use AppleScript to invoke the CLI plugin via the Plugins menu
    PLUGIN_TITLE=$(osascript -e '
        tell application "System Events"
            tell process "MuseScore 4"
                set frontmost to true
                delay 0.5
                -- Click Plugins menu bar item
                tell menu bar 1
                    tell menu bar item "Plugins"
                        tell menu "Plugins"
                            click menu item "m4bon-cli"
                        end tell
                    end tell
                end tell
            end tell
        end tell
    ' 2>&1) || true

    # Wait for plugin to process
    sleep 2

    # Save as result file
    osascript -e '
        tell application "System Events"
            tell process "MuseScore 4"
                set frontmost to true
                delay 0.3
                -- File → Save as...
                tell menu bar 1
                    tell menu bar item "File"
                        tell menu "File"
                            click menu item "Save as..."
                        end tell
                    end tell
                end tell
            end tell
        end tell
        delay 0.5
        tell application "System Events"
            tell process "MuseScore 4"
                -- Type the save path
                keystroke "'"$RESULT"'"
                delay 0.3
                keystroke return
            end tell
        end tell
    ' 2>&1 || true

    # Wait for save
    sleep 2

    # Quit MuseScore
    kill "$MS_PID" 2>/dev/null || true
    sleep 1

    # Inspect result with --score-elements
    if [ -f "$RESULT" ]; then
        echo -e "  ${PASS} (result produced)"
        PASSED=$((PASSED + 1))
        echo "  Score elements (notes/chords):"
        "$MSCORE" "$RESULT" --score-elements 2>/dev/null \
            | python3 -c "
import sys, json
data = json.load(sys.stdin)
for part in data:
    for el in part.get('elements', []):
        if el.get('type') in ('Chord', 'Note', 'Rest') and el.get('duration', {}).get('name') not in ('Measure',):
            print(f'    {el[\"type\"]}: beat={el.get(\"beat\")}, dur={el.get(\"duration\",{}).get(\"name\")}')" 2>/dev/null || echo "    (no notes inserted)"
    else
        echo -e "  ${FAIL} (no result file)"
    fi

    echo ""
done

echo ""
echo "=== Results: $PASSED / $TOTAL passed ==="
