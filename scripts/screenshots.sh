#!/bin/sh
#
#   The README's screenshots: docs/screenshots/de/ and docs/screenshots/en/, each picture light and
#   dark (-dark), taken by ui/e2e/screenshots.spec.ts against a daemon of its own that starts with
#   ui/e2e/shot-seed on the fake system (no real system's names or addresses). Then made small: 256
#   colours (the pictures are flat UI), with sharp installed into a temporary directory - the guide's
#   pictures are in the daemon, so in every package.
#
#     scripts/screenshots.sh
#
#   Needs the page built (cd ui && npm run build) and Chromium for Playwright; on WSL run it with
#   env -u WAYLAND_DISPLAY -u DISPLAY.
#
set -e
cd "$(dirname "$0")/../ui"
LOGIC_SCREENSHOTS=1 npx playwright test e2e/screenshots.spec.ts --project=desktop-light
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
npm install --prefix "$tmp" --no-audit --no-fund --silent sharp@0.33.5
NODE_PATH="$tmp/node_modules" node -e '
const sharp = require("sharp"), fs = require("fs");
(async () => { for (const f of process.argv.slice(1)) {
    const b = await sharp(f).png({palette: true, quality: 90, effort: 10, compressionLevel: 9, dither: 0.6}).toBuffer();
    if (b.length < fs.statSync(f).size) fs.writeFileSync(f, b);
} })();' ../docs/screenshots/*/*.png
du -ch ../docs/screenshots/*/*.png | tail -1
