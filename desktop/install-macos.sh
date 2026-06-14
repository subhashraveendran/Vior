#!/bin/bash
# Vior — macOS one-click installer
# Double-click this in Finder, or drag the .app next to this script into
# /Applications manually. This script copies the .app and strips the
# Gatekeeper quarantine flag so you don't need to do anything else.
set -euo pipefail

APP=$(ls -t ./*.app 2>/dev/null | head -1)

if [ -z "$APP" ]; then
  echo "No .app found next to this script."
  echo "Make sure the .app is in the same folder as this installer."
  exit 1
fi

echo "Installing $APP to /Applications…"
rm -rf "/Applications/$(basename "$APP")"
cp -R "$APP" /Applications/
xattr -dr com.apple.quarantine "/Applications/$(basename "$APP")"

echo ""
echo "✓ Vior installed. You can now open it from Launchpad or /Applications."
echo "  (The quarantine warning has been cleared automatically.)"
