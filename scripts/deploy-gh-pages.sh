#!/usr/bin/env bash
# deploy-gh-pages.sh — Deploy web/ to the gh-pages branch for GitHub Pages.
#
# Prerequisite: `make wasm` must have already built web/m4bon.wasm.
# After running: git push origin gh-pages
# Then in repo Settings → Pages: Source = "Deploy from a branch", branch = gh-pages, / (root).
set -euo pipefail

cd "$(dirname "$0")/.."

WEB_DIR="web"
FILES=(
  index.html
  app.js
  app.css
  wasm_exec.js
  WebAudioFontPlayer.js
  m4bon.wasm
  bass-As1.ogg
  bass-Cs2.ogg
  bass-E1.ogg
  bass-G1.ogg
)

# Verify all files exist
for f in "${FILES[@]}"; do
  if [ ! -f "$WEB_DIR/$f" ]; then
    echo "ERROR: $WEB_DIR/$f not found. Run 'make wasm' first." >&2
    exit 1
  fi
done

# Copy files to a temp directory while still on the source branch
STAGING=$(mktemp -d)
cleanup() {
  rm -rf "$STAGING"
}
trap cleanup EXIT

for f in "${FILES[@]}"; do
  cp "$WEB_DIR/$f" "$STAGING/"
done

# Save current branch so we can return
CURRENT_BRANCH=$(git branch --show-current)

# Stash any dirty state in the current branch
DIRTY=0
if ! git diff-index --quiet HEAD --; then
  DIRTY=1
  git stash push -m "gh-pages-deploy-auto-stash"
fi

return_branch() {
  git checkout "$CURRENT_BRANCH" 2>/dev/null || true
  if [ "$DIRTY" -eq 1 ]; then
    git stash pop 2>/dev/null || true
  fi
}

# Create or update gh-pages branch
if git show-ref --verify --quiet refs/heads/gh-pages; then
  echo "Updating existing gh-pages branch..."
  git checkout gh-pages
  find . -maxdepth 1 ! -name '.git' ! -name '.' -exec rm -rf {} +
else
  echo "Creating new gh-pages branch..."
  git checkout --orphan gh-pages
  git rm -rf --quiet . 2>/dev/null || true
fi

# Copy web files from staging to root
for f in "${FILES[@]}"; do
  cp "$STAGING/$f" .
  git add "$f"
done

# Commit
if git diff-index --quiet --cached HEAD -- 2>/dev/null; then
  echo "No changes to deploy."
else
  git commit -m "Deploy web TUI to GitHub Pages"
  echo ""
  echo "Deployed to gh-pages branch. To publish:"
  echo "  git push origin gh-pages"
  echo ""
  echo "Then enable GitHub Pages in repo Settings:"
  echo "  Settings → Pages → Source: Deploy from a branch"
  echo "  Branch: gh-pages, / (root)"
fi

# Return to original branch
return_branch