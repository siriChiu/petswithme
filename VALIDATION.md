# Validation status

## Executed in the Linux build environment

- Pure behavior/config/image tests, including long deterministic simulation
- Independent per-cat timing, start/loop/end and interruption generations
- Fallback-cycle/rectangle/duration validation
- Same-monitor social reservation, cancellation, cooldown and collision guards
- Premultiplied BGRA ordering and alpha hit-threshold conversion
- Negative-coordinate monitor bounds, settings and config validation
- Windows x64 GUI cross-compilation without cgo
- Windows x64 Win32 structure/offset compile-time assertions

## Native Windows CI

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
