## v0.1.0 (unreleased)

- initial release: `scan`, `dump`, `listen`, `raw`, and every 1V48 VFD clock control the vendor app exposes (`sync-time`, `set-date`, `set-time`, `brightness`, `alarm`, `display`, `tick`, `transition`, `auto-sync`), plus untested `lamp` commands for the XGGF spectrum lamps. Protocol reverse-engineered from the vendor app, see PROTOCOL.md.

## unreleased notes (live verification 2026-09-07)

- verified against a real XGGF-1V48: `set` writes date/time and the clock updates; the Nordic UART UUIDs (6e400001/2/3) match.
- the clock advertises its name only intermittently, so discovery falls back to a remembered address; `set`/`get` reuse it automatically after the first connect.
- the clock's only notification is a ~3s `0x23` heartbeat; it never reports settings, so `get` shows last-sent values plus a liveness check, not a true read-back.

## unreleased notes (command scheme)

- flat `acornvfd <thing>` commands (sync-time, set-date, set-time, brightness, alarm, display, tick, transition, auto-sync); no get/set grouping, since the clock cannot be read back a `get` would be misleading.
- the remembered device file holds only the clock's address (for reliable reconnection), not settings.
- renamed `set-date`/`set-time` to `date`/`time`; bare `time` syncs date+time (replacing `sync-time`), bare `date` sets today's date.
