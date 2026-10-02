# Pets With Me / 三貓桌面陪伴

A new, offline Windows desktop companion. Three independent cats live above the
desktop, watch the pointer, respond to petting, choose random reachable destinations,
and coordinate short approach, follow/chase, and rest interactions when their
artwork supports those actions.

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
- Double-click: one complete play / jump sequence, including landing
- Hold and drag: move; interrupts autonomous behavior
- Right-click a cat or its notification-area icon: menu
- Activity: quiet / normal / lively; livelier cats decide and explore more often
- Quiet mode: stops autonomous activity; direct interactions still work
- Hide/show, reset to pointer's monitor, three sizes, play-all, and quit are in the menu
- Three minutes of system inactivity: settle into an available sleep/sit/rest pose, otherwise idle

The build is unsigned. Windows may show SmartScreen or antivirus warnings. Do
not disable security software. Code signing and a full recipient-machine check
are still release tasks. See [VALIDATION.md](VALIDATION.md) for tested versus
unverified behavior.

## Privacy

The program only samples cursor position, the primary mouse-button state while
dragging, aggregate system idle duration, and aggregate system CPU timing at roughly two-second intervals. It does not capture screens,
read window titles/documents, record keystrokes, make network requests, install
services, modify startup settings, or download updates. Small size/activity/CPU-response settings
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
- `autonomy.go` / `activity.go`: weighted choices, destination/dwell planning, artwork gates and activity preferences
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

Normal and lively settings alter decision frequency, never silently speed up a
walk animation. Running requires distinct run artwork and explicitly verified
stride metadata. Sit/get-up, stretch, groom and pounce choices stay disabled when
the pack lacks directly authored frames. A fallback is not another finished
action. Pair greetings are skipped if unavailable; joint rest uses only an
authored resting pose, or idle. The app does not claim that these code paths
mean the private art is complete.

Optional per-cat `temperament` values (energy, sociability, curiosity; 0–1)
control local play preferences. Defaults differ between slots; these are not
inferred personality claims about real pets. Roaming uses reachable horizontal
destinations, pauses, cooldowns and repeat avoidance. When idle, authored gaze poses continuously follow the global mouse direction,
including quiet mode. This presentation never postpones a roaming decision.
Directional art must be supplied; neighboring gaze aliases are explicit.
A small face-centered dead zone and angular hysteresis prevent boundary flicker.
Petting, dragging, movement, CPU actions, and inactivity sleep take priority.

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

For walking review with visible ground ticks and real application translation:

```
go run -tags motionpreview . --root /path/to/private-pack --out /path/to/empty-gait --gait-only
```

This defaults to 20fps, one lane per character. Metadata includes exact and
rounded root positions, source frame indices, declared stride calibration, and
within-pose hold travel. Missing/static walk aliases stay still; anatomical gait
and likeness still need visual review. Do not calibrate a bad source loop merely
to make format tests green.

To inspect a separate run candidate, use `--gait-only --gait-actions run_right,run_left`.
Unverified runs show explicitly labeled trial translation for sole-contact
measurement; they are not enabled for autonomous movement in the app. The
metadata reports that distinction.

## CPU-responsive kneading

The optional CPU response is enabled by default and can be turned off in the
notification-area menu. It reads only aggregate system timing using Windows
`GetSystemTimes`, with no process inspection, window contents, or keystroke
monitoring. CPU measurements stay in memory. This indicates computer load,
not whether the person is busy or working.

Defaults: smoothed CPU above 70% for 10 seconds starts stationary `knead`;
smoothed CPU below 50% for 10 seconds ends it. Smoothing uses a four-second
time constant and a two-second sampling gate. While latched busy, the 50–70%
band avoids rapid switching. Each cat stretches once per 300 seconds of active
busy behavior, then resumes kneading. Direct actions and quiet mode pause that
cat's elapsed busy-action time; they never queue catch-up stretches.

Drag, petting and manual play outrank the response. Quiet mode disables it.
Supported CPU response outranks automatic inactivity sleep, so a long unattended
computer job can still show the five-minute stretch. Hide/show, reset, power
suspend/resume, failed samples or gaps longer than six seconds require fresh
continuity. No sampling takes place while hidden or disabled. Active busy mode
uses a modest 100ms render timer; dragging retains its normal responsive timer.

Only directly authored, distinct `knead` and `stretch` loops qualify. Missing
knead art leaves normal behavior; missing stretch art keeps kneading. A copy of
idle/pet frames does not qualify. This code support does not mean those private
generated artwork packs are ready.

Settings in `%APPDATA%/ThreeCatCompanion/settings.json` include a `cpu` object
with `enabled`, `enterPercent`, `exitPercent`, `enterSeconds`, `exitSeconds`, and
`stretchEverySeconds`. Close the app before editing thresholds/timing. Defaults
are true / 70 / 50 / 10 / 10 / 300 respectively. Durations are in seconds and
both thresholds must be strictly between 0 and 100, with exitPercent lower than enterPercent. Invalid values safely use defaults,
while preserving an explicit disabled setting.

`GetSystemTimes` includes idle in its kernel counter. The sampler subtracts idle
from checked kernel+user deltas. On a machine with multiple processor groups it
fails closed rather than calling a partial-group measurement whole-machine CPU.
Ordinary pet behavior keeps working. See Microsoft's [GetSystemTimes API](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getsystemtimes)
and [processor-group API](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getactiveprocessorgroupcount).

## Optional experimental stylized movement

`experimentalMovement` is off by default. A private `cats.json` may explicitly
opt in for a user-approved playable preview; a saved preference takes priority.
The tray checkbox is labeled as uncalibrated with possible foot sliding, and an
explicit saved off choice remains off on later launches.

This option permits only directly authored, distinct run frames, at a conservative
0.23 canvas-widths/second trial speed. It does not change `movement.verified`,
claim natural gait, synthesize frames, or accelerate a walk clip into a run.
Verified run calibration remains unchanged. Missing/idle-alias runs are still
unavailable. Headless gait previews report this mode separately from calibration.

To compare the same actions across all supplied cats, including complete recovery
frames, use `--actions idle,pet,drag,play,knead,stretch,greet,idle`. This mode shows
entry, at least one full loop, and every end frame before changing scenes. Its
metadata records each sampled phase and source rectangle, plus per-cat authored
versus fallback capabilities. It counts distinct rendered gaze drawings separately
from usable direction aliases. These pixel/reference checks do not certify pose
anatomy or likeness; visual review is still required. Keep the report private when
it contains private names or asset paths.

## Work-friendly controls

Single-click the notification-area icon to hide/show the cats. Right-click it
for Settings or temporary whole-pet click-through. Click-through automatically
ends after five minutes, can be ended immediately from that same tray menu, and
is never persisted across restarts. Transparent margins retain normal per-pixel
click-through when this temporary mode is off.

The native Settings window adjusts size, activity and CPU thresholds/timing.
Invalid ranges or an exit threshold above the entry threshold are rejected;
Cancel leaves preferences unchanged. Applying saves locally, then updates the
existing engine. The dialog is an explicitly opened normal window; pet windows
remain nonactivating. No global keyboard shortcut or key monitoring is installed.

The app retains its low-resource 50ms normal timer, 100ms busy timer, 250ms quiet
idle timer and 16ms drag timer. Deliberate pet/play interactions temporarily use
50ms even in quiet mode, through their full recovery; idle then returns to 250ms.
Additional genuine animation poses subdivide existing
clip durations rather than speeding up playback. A higher repaint rate is not a
substitute for extra authored drawings.

Locomotion timing now moves only during displayed loop time. Optional entry and
recovery frames remain stationary; arrival and direction changes finish a declared
recovery before the next motion clip. This removes an extra first-tick slide and
skipped recovery poses. It does not correct inconsistent paw trajectories in the
artwork, and reused contact holds are not newly drawn transitions.

Explicit play/jump commands play one complete clip rather than repeating a short
jump for a fixed five seconds. The recovery is presented before returning to idle;
delayed timer delivery cannot expire the action before its first visible pose.
Repeated deliberate commands restart it, and dragging still interrupts at once.

Private packs can declare a reference width when adding transparent room for a
horizontal tail. The native size preference and DPI keep the existing body scale;
window bounds, drag/hit testing and cached rendering use the wider physical
canvas. Legacy packs retain their previous size arithmetic.

A press freezes autonomous motion immediately, but pickup artwork begins only
after the pointer crosses the existing drag threshold. Ordinary clicks and
double-clicks therefore avoid flashing a pickup pose before petting or jumping.
Capture cancellation, hiding and temporary click-through release pressed state.

Isolated frame-deadline experiment: normal-mode updates may wake at the next
pose deadline while retaining a maximum nominal 50 ms movement interval. It does
not change image files, authored durations, posture ordering or travel speed.
Earlier armed wakes are preserved when settings callbacks reschedule. Quiet,
busy, dragging and hidden timer policies are retained; no system timer-resolution
change is requested. Windows delivery is best effort, so this is not a 60 FPS
promise or a remedy for missing/inconsistent poses. The Windows test records
actual SetTimer/message-loop cadence, visible pose holds, update counts and
process CPU time for baseline/adaptive runs with synthetic moving windows.
