# Validation status

## Executed in the Linux build environment

- Pure behavior/config/image tests, including long deterministic simulation
- Independent per-cat timing, start/loop/end and interruption generations
- Fallback-cycle/rectangle/duration validation
- Same-monitor social reservation, cancellation, cooldown and collision guards
- Seeded random destinations, dwell variation, repeated-action avoidance and activity-rate simulations
- Real-action gates, separate verified running, quiet cancellation and sit/get-up end transitions
- Legacy quiet-setting migration and per-cat temperament validation
- Premultiplied BGRA ordering and alpha hit-threshold conversion
- Negative-coordinate monitor bounds, settings and config validation
- Windows x64 GUI cross-compilation without cgo
- Windows x64 Win32 structure/offset compile-time assertions

## Native Windows CI

Observed pass on 2026-10-01: [run 36811633388](https://github.com/siriChiu/petswithme/actions/runs/36811633388), Windows Server 2025. Both native lifecycle and per-pixel alpha-hit tests executed and passed, not skipped. The private-art test was intentionally skipped in the public repo; it passes locally with the private demo atlas.

The subsequent full-app subprocess test runs real main, config/PNG loading, tray setup, three pets, quiet/normal/lively/hide/show/reset/play/size/quit and cleanup, with isolated preferences and synthetic art. It reports an explicit skip only if the runner lacks an interactive tray desktop.

The workflow runs tests on windows-latest using synthetic in-memory QA pixels,
not private cat art. It exercises layered window creation/update, styles,
nonactivation, show/hide, window bounds and native resource cleanup. A separate
alpha test checks WindowFromPoint for opaque/transparent pixels and the underlying
probe window. That specific interactive test explicitly reports SKIP if a visible
input desktop is unavailable. Inspect the actual CI run before treating it as a
pass; cross-compilation alone is not a native-runtime result.

## Still required on an actual recipient-like desktop

- Click an ordinary app through transparent sprite margins
- Fast repeated drag, capture interruption, swapped mouse buttons
- Drag between 100%, 150%, 200% DPI monitors, including negative monitor origins
- Unplug a monitor and recover all cats with Reset
- Explorer restart and notification-area menu recovery
- Suspend/resume, locking/unlocking, taskbar movement
- Long idle CPU/memory measurement and clean whole-app quit
- Final three approved character assets and motion quality
- Code signing / SmartScreen experience

The private demo is a test build, not an already-validated finished gift.
