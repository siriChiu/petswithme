# Artwork and animation format

Artwork is supplied independently of behavior logic. Files stay local and are
never uploaded by the app. A PNG has a 24 MiB compressed and 8-million-pixel decoded
limit. Configuration paths must be local, relative paths; maximum three cats.

## Legacy demo atlas

8 columns × 11 rows. Every used frame must contain nontransparent pixels. Rows:

| Row | Meaning | Used frames |
|---|---|---:|
| 0 | idle / blink | 6 |
| 1 | move right | 8 |
| 2 | move left | 8 |
| 3 | wave / pet feedback | 4 |
| 4 | jump | 5 |
| 5 | disappointed | 8 |
| 6 | waiting / expectant | 6 |
| 7 | seated busy action | 6 |
| 8 | look around | 5 |
| 9 | gaze up clockwise to down-right | 8 |
| 10 | gaze down clockwise to up-left | 8 |

The sixteen gaze indices are viewer coordinates: 0 up, 4 right, 8 down, 12 left.
Idle tracking uses the global screen pointer relative to the upper-body point
(window center x, 30% height y), with no proximity timeout. Quiet mode can still
look around without moving. Missing gaze art stays idle; aliases to neighboring
authored gaze directions are allowed but are not sixteen distinct drawings.
The demo sleeping action holds an existing closed-eye idle frame. Groom/stretch
and social actions use explicitly marked fallback art until dedicated art exists.

## Custom manifest

Add `"animations": "art/cat-animation.json"` alongside a cat's `sprite` path in
private cats.json. Source rectangles support arbitrary sprite layouts, rather
than requiring 8×11. Mixing legacy row/col frames requires a compatible 8×11 image.
The canvas fits the largest declared frame; frames retain aspect ratio.

```json
{
  "schemaVersion": 1,
  "fallback": "idle",
  "anchor": {"x": 0.5, "y": 1},
  "actions": {
    "idle": {
      "loop": [
        {"rect":{"x":0,"y":0,"w":192,"h":208},"durationMs":250},
        {"rect":{"x":192,"y":0,"w":192,"h":208},"durationMs":250}
      ]
    },
    "pet": {"fallback":"idle", "demoFallback":true}
  }
}
```

Each action supports start, loop, end frame arrays, a fallback action, and mood
variants (`moods`). Each frame supports durationMs (10–60,000), either row/col or
rect, and an optional normalized anchor override. All rectangles are validated
against image bounds; invalid actions/fallback cycles fail loading with a clear
error. Missing requested actions use the manifest's declared global fallback.

Behavior names: idle, walk_left, walk_right, run_left, run_right, drag, pet, play,
sleep, sit, getup, rest, pounce, knead,
greet, social_rest, curious, waiting, groom, stretch, and gaze_0 through gaze_15.
Each cat has its own animation cursor and interruption generation, even when art
is shared. An explicit play/jump command traverses start, one loop, then end;
it does not repeatedly replay a full takeoff-and-landing loop for a fixed timeout. Explicit actions finish their end phase; urgent interruptions replace
it immediately. Anchor defaults should normally be bottom-center for feet.

## Approval pipeline

Approve each cat's identity first, then draw action families using that approved
reference. Never infer three unique cats from one demo atlas. Keep photos, names,
unapproved previews, and finished private artwork out of the public repository.

## Walking calibration

A real walk action may include `"movement":{"strideRatio":0.3}`. The ratio is
**full rendered canvas widths travelled per complete loop**, measured from the
corrected source art's planted-paw trajectory; it is not a speed guessed from
appearance. The engine computes pixels/second from canvas width × strideRatio ÷
loop duration, so frame timing changes and display scaling stay coupled.

Only elapsed time in an already displayed locomotion loop translates the root.
Optional start/end clips are stationary; no displacement is inferred for them.
A new direction first shows its entry pose. If the old clip declares an end,
turning finishes that recovery before starting the opposite direction. Arrival
also preserves the end sequence. Dragging, petting, quiet mode and other higher
priorities still interrupt immediately. A pack may explicitly reuse a grounded
loop frame for a short stop hold, but this is not additional transition artwork.

Without explicit calibration, genuine multi-frame walking keeps the conservative
legacy speed and is reported as uncalibrated by the preview tool. Missing,
single-pose, or known idle/sit/sleep fallback walks do not move the cat. Distinct
frame references alone cannot prove anatomical gait quality: review contact,
passing, lift-off, swing, and loop closure, anchored by shoulders/hips rather than
a changing tail silhouette. Report sampled planted-paw drift separately from the
within-frame hold jitter of sprite animation. Never call a near-static loop a
validated walk just because format checks pass.

`movement.verified` defaults to false. A trial stride may be rendered without
claiming it is calibrated. Set verified=true only after separate planted-paw
review; the renderer reports the declaration and does not independently certify
anatomical gait.

## Ground contact and transparent padding

For custom-rectangle actions other than drag, the source anchor is a reference
point mapped to the bottom-center of the pet window. It is not merely a
letterboxing preference. Set its y to the shared source contact plane after
hip/shoulder registration, so transparent padding does not make pets hover.
Intentional airborne motion retains that same ground reference; do not recenter
every jump or stance frame. Drag preserves its full canvas to protect dangling
tails. Validation rejects ground alignment that would clip visible pixels.
The same composition and recorded source-to-window transform are used in the
native app and motion previews. Keep explanatory notes in separate QA files;
unknown manifest fields are rejected.

## Autonomous action availability

The engine schedules only directly authored actions (`loop` or a mood loop, with
`demoFallback` false). An alias to a different action does not qualify as new
artwork. Neighboring gaze aliases are the narrow exception. All mandatory
interaction priorities and fallback validation remain in effect. A missing
resting pose produces idle, rather than claiming to sleep or groom.

A run needs its own `run_left` / `run_right` loop with at least two distinct
references, not just the walk references at shorter durations. It also requires
its own `movement.strideRatio` and `movement.verified: true`. This is a scheduling
gate, not automated visual approval: the pack author must review anatomy and
visible sole contacts before declaring verification. Unverified trial run art
can be inspected separately but is not autonomously enabled.

`cats.json` may include per-cat `temperament: {"energy":0.6,"sociability":0.5,
"curiosity":0.7}`. Each value must be finite and between 0 and 1. Activity and
temperament change choice probabilities and pause durations, not frame timing or
calibrated stride speed. Old settings with only `quiet` continue to load.

## CPU response artwork

`knead` is stationary alternating-paw kneading. `stretch` is a separate whole-cat
stretch with its own start/loop/end phases. Both need directly authored clips and no `demoFallback` marker. Knead needs
at least two distinct loop references; stretch needs distinct poses across its
start/loop/end sequence and may hold one deep-stretch loop frame. Repeating
an idle/pet/walk loop under either name is rejected as a capability. Source
reference checks do not replace visual verification of actual drawn poses.

The load state never changes animation playback speed or calibrated stride. It
keeps the cat's root stationary, pauses for direct interaction and quiet mode,
and consumes only one stretch opportunity per configured interval. Missing
stretch artwork leaves kneading in place.

When padding changes to contain a tall pickup or a wide stretch, keep the art's
scale and ground reference unchanged. An optional top-level
`"gazeOrigin":{"x":0.5,"y":0.43}` declares the visible head's normalized point
within the final window canvas; both values must be finite and in [0,1]. Omitting
it preserves the original center-x/30%-height behavior. This point is independent
of the floor anchor. Review the real idle-to-action transition after any canvas
change, including gaze directions close to the face; do not fit each silhouette
independently or silently shrink the character to fit a cell.

A top-level `gazeDirections` may be 4, 8, 16 or 32. Omitted/zero means 16 for
backward compatibility, including older four-cardinal packs with sixteen aliases.
Within the declared count, `gaze_0` starts at up and indices proceed clockwise in
`360/count` degree increments. For 32: up=0, right=8, down=16, left=24. Merely
changing this number does not create art: missing indices still fall back, and
capability reports count genuine distinct rendered drawings separately. Existing
16-direction art can be retained at even indices when real generated intermediate
poses are added at odd indices. The mixed-pack preview uses the same screen angle
for cats with different counts.

## Lossless transparent-margin packing

A custom-rectangle frame may include a logical canvas:
`"canvas":{"width":320,"height":288,"offsetX":64,"offsetY":80}`.
The stored `rect` contains only the cropped source pixels; offsetX/Y restore its
position within the original logical canvas. Logical sizes are bounded to
1–4096 and the crop must fit completely inside it. Rendering, anchors, gaze
origins and pet-window dimensions use the logical canvas, not the packed crop.
Alpha pixels, nearest-neighbor sampling and animation timing remain identical.
This allows denser generated pose sets without increasing the 8MP decoded-image
cap, reducing character size, or discarding genuine frames. Strip every fully
transparent margin only after preserving the complete generated silhouette.

For explicitly enabled uncalibrated trial runs, optional movement
`trialSpeedRatio` (0.05–0.5 canvas widths per second) replaces the conservative
0.23 default. It affects only root travel, never pose duration or the verified
flag. This permits normal/slow real-engine comparisons while revising generated
gaits. Verified stride calibration always takes precedence. A trial speed is not
proof of planted paws or a natural gait, and unverified running still requires
the user's experimental-movement opt-in.

Optional top-level `referenceWidth` (16–4096 source pixels; omitted is legacy)
lets a wider transparent canvas retain the existing character scale. For example,
384×288 with referenceWidth 320 uses a 230×172px window at the 192px size setting;
the same body pixels keep approximately the old scale. All sizes and monitor DPI
use this reference. Center existing frames consistently in the wider canvas and
update gazeOrigin/anchors to the new normalized coordinates. Movement stride and
trial-speed ratios still use full canvas widths: rebase old ratios when padding
changes, so extra transparent room does not accelerate an existing gait.

Partial gaze packs can opt into `"gazeNearestAuthored":true`. Pointer selection
then uses the closest angle that has its own non-demo default or current-mood
loop. Alias-only entries do not create directions. Angular hysteresis scales
with the actual neighboring angle gap; radial hysteresis still applies. This
preserves old angle boundaries where no in-between art exists. The option is
off by default for backward compatibility. Metadata separately reports declared
index count, direct authored indices, and distinct rendered drawings.
