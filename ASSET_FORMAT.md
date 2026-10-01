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

Behavior names: idle, walk_left, walk_right, drag, pet, play, sleep, sit, rest,
greet, social_rest, curious, waiting, groom, stretch, and gaze_0 through gaze_15.
Each cat has its own animation cursor and interruption generation, even when art
is shared. Explicit actions finish their end phase; urgent interruptions replace
it immediately. Anchor defaults should normally be bottom-center for feet.

## Approval pipeline

Approve each cat's identity first, then draw action families using that approved
reference. Never infer three unique cats from one demo atlas. Keep photos, names,
unapproved previews, and finished private artwork out of the public repository.
