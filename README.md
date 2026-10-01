# Pets With Me / 三貓桌面陪伴

A new, offline Windows desktop companion. Three independent cats live above the
desktop, watch the pointer, respond to petting, wander, and coordinate short
approach → greet → follow → rest interactions.

This is an early prototype. The public repository contains **code and synthetic
QA fixtures only**. No reference photos, private cat artwork, or artwork-bearing
release binary is published here. Private builds load approved character artwork from separate local packs. The
initial demo used one atlas in three labeled slots; later packs remain private.
A code build does not imply that every animation has bespoke artwork.

## Run a private demo

Unzip the complete private demo folder and double-click `ThreeCatCompanion.exe`.
Windows 10/11 x64 is the intended target. No Python, Node, .NET or ChatGPT install,
account, API key or network connection is required at runtime.

- Click a cat: pet / wave response
- Double-click: short play / jump sequence
- Hold and drag: move; interrupts autonomous behavior
- Right-click a cat or its notification-area icon: menu
- Quiet mode: stops wandering and social activity; direct interactions still work
- Hide/show, reset to pointer's monitor, three sizes, play-all, and quit are in the menu
- Three minutes of system inactivity: settle into a demo sleeping pose

The build is unsigned. Windows may show SmartScreen or antivirus warnings. Do
not disable security software. Code signing and a full recipient-machine check
are still release tasks. See [VALIDATION.md](VALIDATION.md) for tested versus
unverified behavior.

## Privacy

The program only samples cursor position, the primary mouse-button state while
dragging, and aggregate system idle duration. It does not capture screens,
read window titles/documents, record keystrokes, make network requests, install
services, modify startup settings, or download updates. Small size/quiet settings
are saved under `%APPDATA%/ThreeCatCompanion/settings.json`. Delete that folder to
reset preferences. Hidden mode stops the animation timer entirely.

## Build

Requires Go 1.25.12 or newer to build; the recipient does not need Go.

```
go test ./...
# Windows PowerShell
./scripts/build.ps1
# Linux/macOS shell
./scripts/build.sh
```

The pure-Go/Win32 build cross-compiles without cgo. The public source also compiles
without private art, but that binary is a source-check artifact: launch explains
that artwork is required. For a usable private demo, place an authorized v2 PNG at
`assets/spritesheet-extended.png` before building, or provide custom sprite and
animation paths in a private `cats.json`. Never commit private artwork or names.

Copy `cats.example.json` to `cats.json` beside the executable. `sprite: ""` uses the
embedded demo. A relative `sprite` path loads a PNG inside the app folder; optional
`animations` points to an animation JSON file. One to three cats are supported.

## Architecture

- `core.go`: bounded image/config loading, geometry and premultiplied BGRA conversion
- `asset_validation.go`: frame-pixel validation, pack path boundaries and stable initial anchors
- `behavior.go`: independent animation players, action arbitration and social coordinator
- `main_windows.go`: Win32 per-pixel-alpha layered windows, tray and pointer events
- `main_preview.go`: optional headless rendering with the same animation player
- Tests: deterministic behavior simulations, format/fallback validation, ABI checks,
  Windows native lifecycle and alpha hit testing with synthetic artwork

The engine is a new implementation. VPet was reviewed for design ideas such as
start/loop/end phases, action fallbacks and interruption priorities; no VPet code
or assets are copied or linked. It is not a VPet fork or a ChatGPT pet plugin.

Each cat owns its behavior state and animation cursor. Immutable decoded images
can be shared; rendered frame cache is bounded at 32 MiB. Dragging, petting, and
explicit play cancel conflicting social reservations. Quiet mode keeps direct
controls available. A single clock drives all animation and movement.

## Artwork contract

See [ASSET_FORMAT.md](ASSET_FORMAT.md). Artwork identity approval and animation
quality are separate from engine testing. Missing actions deliberately reuse a
declared fallback; the app does not synthesize new art or pretend that a fallback
is a finished bespoke animation.

## License

Project source: MIT, see [LICENSE](LICENSE). Private artwork is not included and
is not licensed by the source-code license.

## Private motion previews

The optional headless tool renders only supplied artwork, using the real animation
player at the default 144px width. It refuses demo/missing packs and writes a PNG
sequence plus timing/fallback metadata. No private input or output belongs in git.

```
go test -tags motionpreview ./...
go run -tags motionpreview . --root /path/to/private-pack --out /path/to/empty-preview
```

Default scenes cover idle, both walks, pet, drag, play, sleep and a gaze sweep.
Missing actions retain their explicitly reported fallback chain; a preview is not
proof that bespoke art exists for every action. Preview metadata may include private
names and local paths, so keep it with the private pack.
