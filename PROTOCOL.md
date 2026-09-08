# XGGF BLE protocol (橡果工坊 / Acorn Workshop, diym.vip)

Reverse-engineered from the vendor Android app `XGGF.apk`
(downloaded 2026-09-07 from https://diym.vip/upload/XGGF.apk, 20.5 MB).

The APK is a **uni-app (DCloud) bundle**: all device logic is JavaScript in
`assets/apps/__UNI__971C86D/www/app-service.js`, not in the DEX files. The same
uni-app source is almost certainly what the vendor compiles to the WeChat
mini-program 橡果易控, so the protocol below should match what WeChat sends.
Source line references like `common/ble.js:1340` come from the app's own debug
log strings and refer to the vendor's original (unminified) source.

Legend: **[verified in code]** = read directly from the app source.
**[guess]** = my inference, not confirmed. **[TODO live]** = confirm against the
real device.

---

## 1. Products covered by the app

| Advertised BLE name        | App `detailType`    | Control page                 | Protocol           |
|----------------------------|---------------------|------------------------------|--------------------|
| `XGGF-1V48`                | `fluorescent_clock` | `Dev2025_FluorescentClock`   | binary, see §3–4   |
| `XGGF-XW25`                | `spectrum_xw25`     | `Dev2025_Spectrum`           | binary, see §5     |
| `XGGF-8W25`                | `spectrum_8w25`     | `Dev2025_Spectrum`           | binary, see §5     |
| `XGGF-8W30-PRO`            | `spectrum_8w30_pro` | `Dev2025_Spectrum`           | binary, see §5     |
| `XGGF-IN1204/1206/1404/1406` (prefix, first 11 chars) | `in12_4` … | `Dev2025_IN_XGGF`  | legacy ASCII, see §6 |
| (none)                     | `pixel_clock`, `nixie_clock` | product catalogue only | no BLE code present |

- The app only considers scan results whose `localName`/`name` starts with
  `XGGF` **[verified in code]** (`common/ble.js:331`).
- **[verified live 2026-09-07] The clock advertises its name only intermittently.**
  Most advertisement packets carry no local name at all; `XGGF-1V48` appears only
  in the occasional scan response. So name-prefix matching is unreliable: the same
  clock is seen as `(no name)` in most scans. Match by **address** instead, which
  is present in every advertisement. `acornvfd` remembers the address of the last
  device it connected to and reuses it automatically.
- **[observed live] The clock stops advertising for a while after a disconnect.**
  Right after a connect/disconnect it often cannot be found again for tens of
  seconds to minutes. Back-to-back connections frequently fail; the vendor app
  avoids this by holding one connection for the whole session. Do all the writes
  you need in one connection, or expect to wait/retry.
- The IV48 round-dial VFD clock is `XGGF-1V48` (the app spells it with a
  digit one, "1V48"; the UI calls it 荧光时钟 = "fluorescent clock")
  **[verified in code]**. Matching is exact after trim + uppercase.
  **[verified live 2026-09-07]** the clock advertises exactly `XGGF-1V48`
  (no service UUIDs in the advertisement, RSSI about -80 at desk range).

---

## 2. GATT layer ("default" profile)

The app hardcodes no full 128-bit UUIDs. It discovers services and picks
characteristics with these rules **[verified in code]**
(`common/ble.js` ~1294, helpers `dt`/`ut`/`ot`):

1. Ignore services whose 16-bit short UUID is `1800` or `1801` (GAP/GATT).
2. Sort remaining services so any whose normalised UUID starts with
   `6e400001` come first, then take the first service that has a writable
   characteristic.
3. Within that service:
   - **write characteristic**: the one whose UUID starts with `6e400002` and
     has the `write` property; otherwise the first characteristic with `write`.
   - **notify characteristic**: the one starting with `6e400003` with `notify`;
     otherwise the first with `notify`. (Recorded but **never enabled** for the
     clock, see below.)
   - **read characteristic**: first with `read` (optional, unused).
4. Writes use uni-app's default `writeType`, i.e. **write with response**
   (`common/ble.js:1542`). No MTU negotiation, no chunking (packets are 8 bytes).

`6e400001/2/3` is the well-known **Nordic UART Service (NUS)** prefix, so the
expected full UUIDs are **[guess, standard NUS values — the app only matches the
first 8 hex digits]**:

| Role            | UUID                                   |
|-----------------|----------------------------------------|
| Service         | `6e400001-b5a3-f393-e0a9-e50e24dcca9e` |
| RX (phone→dev)  | `6e400002-b5a3-f393-e0a9-e50e24dcca9e` |
| TX (dev→phone)  | `6e400003-b5a3-f393-e0a9-e50e24dcca9e` |

**[verified live 2026-09-07]** `acornvfd dump` on the real clock shows exactly one
service, `6e400001-b5a3-f393-e0a9-e50e24dcca9e`, containing `6e400002` (RX) and
`6e400003` (TX), MTU 512, both "reading is not permitted". No 1800/1801, no other
services. The full UUIDs above are therefore confirmed.

**Observed live [needs more testing]:** after one connect/disconnect cycle (the
`dump` above) the clock **stopped advertising** and was not seen again in ~3 minutes
of scanning. Either it re-advertises only after a longer timeout, or it needs a
power-cycle, or CoreBluetooth kept a link open. The vendor app keeps a connection
alive for the whole session, which would hide this. Work-around until understood:
do everything you need in one connection, or power-cycle the clock between runs.

**The clock page is write-only.** The app never calls
`notifyBLECharacteristicValueChange` for the default profile (only for the
legacy nixie profile) and never parses any bytes from the clock
**[verified in code]**. So there is no known status/readback command.

**[verified live 2026-09-07] The clock does emit notifications the app ignores,
but only a heartbeat.** Subscribing to `6e400003` yields a single byte `0x23`
(ASCII `#`) roughly every 2.8 s, steady, unaffected by any write. Over a 40 s
capture: 14 heartbeats, nothing else. Sending unused command groups (`FF 00 …`,
`FF 02 …`, `FF 03 …`, `FF 06 …`) produced no distinct reply, only the ongoing
heartbeat. So there is still no way to read a setting back; the heartbeat is only
a liveness signal. A real `get` is therefore impossible; `acornvfd get` reports
the last values it *sent* (from a local state file) plus this heartbeat as an
"alive" check.
The device may still send something on the TX characteristic; **[TODO live]**
`acornvfd listen` (or `get`, which listens for 3 s) and see if anything arrives
after a write.

Connection flow used by the app **[verified in code]**: `createBLEConnection`
→ wait ~300 ms → `getBLEDeviceServices` (retries up to 6× if empty) →
`getBLEDeviceCharacteristics` → cache profile → write. Nothing is written
automatically on connect (no handshake, no password) for this profile.

---

## 3. Packet format (fluorescent clock and spectrum lamps)

Every command is exactly **8 bytes** **[verified in code]**
(`va()` / `oa()` in the bundle):

```
byte 0   0xFF            fixed header
byte 1   group           command group
byte 2   cmd             sub-command (or on/off flag for some toggles, see §4)
byte 3-6 p1 p2 p3 p4     parameters, unused ones are 0x00
byte 7   checksum        (0 - (b0+b1+...+b6)) & 0xFF
```

Checksum: two's-complement of the byte sum, so that **all 8 bytes sum to 0
mod 256**. Every byte is masked with `& 0xFF` before summing.

Python reference:

```python
def packet(group, cmd, p1=0, p2=0, p3=0, p4=0):
    body = [0xFF, group, cmd, p1, p2, p3, p4]
    return bytes(body + [(-sum(body)) & 0xFF])
```

Example: set time 15:23:00 → `FF 01 01 0F 17 00 00` sum = 0x127 → checksum
`(-0x127) & 0xFF = 0xD9` → `FF 01 01 0F 17 00 00 D9`.

---

## 4. Fluorescent clock commands (`XGGF-1V48`)

All **[verified in code]** unless noted. Source:
`pages/device-control/Dev2025_FluorescentClock.vue` (bundle lines ~4287–4500
in the prettified `app-service.js`).

### Group 0x01 — date/time

| Command  | Bytes (before checksum)               | Notes |
|----------|---------------------------------------|-------|
| Set date | `FF 01 00 YY MM DD 00`                | `YY = year - 2000`. App clamps year 2000–2555, month 1–12, day 1–31, but `YY` is masked to one byte so years past 2255 wrap. |
| Set time | `FF 01 01 hh mm ss 00`                | 24-hour, **local** wall-clock time (app uses `new Date()` getters, no time-zone handling). |

**[verified live 2026-09-07]** `acornvfd set` wrote the date and time and the
clock's display updated to the correct wall-clock time. The clock plays a short
**audible beep** when it accepts a command (each write, not an error tone), so a
normal date+time sync produces one or two beeps.

The app sends date and time as **two separate packets** from two separate
buttons ("同步日期" / "同步时间"); it does not auto-sync on connect. Day-of-week
is not sent; the device presumably derives it **[guess]**.

### Group 0x04 — alarm

| Command       | Bytes                                | Notes |
|---------------|--------------------------------------|-------|
| Alarm enable  | `FF 04 01 00 00 00 00`               | Note: the on/off flag sits in the **cmd byte** (`FF 04 <0|1> …`). |
| Alarm disable | `FF 04 00 00 00 00 00`               | |
| Set alarm     | `FF 04 02 idx hh mm rep`             | UI always sends `idx = 0`, `rep = 0`. Default UI value 08:00. |

`idx` (alarm index) and `rep` (repeat) exist as parameters in the builder
function but the UI never sets them to anything but 0. Whether other indices
or repeat masks are supported is **[guess/unknown]**.

### Group 0x05 — display / brightness

| Command                 | Bytes                     | Notes |
|-------------------------|---------------------------|-------|
| Auto-brightness off/on  | `FF 05 <0|1> 00 00 00 00` | Flag in the cmd byte, same pattern as alarm enable. |
| Manual brightness       | `FF 05 02 L 00 00 00`     | `L = round(slider_percent / 100 * 7)` → **0–7**. Slider default 68 % → L=5. Sending this only flips the app's local auto-brightness toggle to off; it does **not** send `FF 05 00…` first. |
| Display mode (显示模式, "analog clock" in code) | `FF 05 03 <0|1> 00 00 00` | UI default: on (1). Presumably 1 = round/analog dial rendering, 0 = digital **[guess]**. |
| Second tick sound (秒钟提示音) | `FF 05 04 <0|1> 00 00 00` | UI default: off. |
| Hour transition animation (小时过渡动画) | `FF 05 05 <0|1> 00 00 00` | UI default: on. |
| Auto time sync (自动校时) | `FF 05 06 <0|1> 00 00 00` | UI default: off. What the device does with this is **unknown**; the app never sends time in response to anything, so it is not an "app pushes time" feature. **[guess]** firmware-side (e.g. GPS/RTC/other module). |

Timing: the brightness slider is debounced 120 ms in the app. The clock page
inserts no delay between successive packets, but the spectrum page waits 80 ms
between a mode-select and a mode packet. Recommendation **[guess]**: leave
≥ 50–100 ms between packets.

### Unknowns / not present in the app

- No command to **read** the current time, brightness, or settings.
- No 12/24-hour, date-format, time-zone, or power-off command in this page.
- Groups 0x02, 0x03, 0x06+ are never used for the clock; whether the firmware
  accepts more is unknown. If you want to probe, keep the same checksum rule.

---

## 5. Spectrum lamp commands (`XGGF-XW25`, `8W25`, `8W30-PRO`) — not clock

Same 8-byte framing and checksum; always **group 0xE0 (224)** **[verified in code]**:

| Command          | Bytes                                | Notes |
|------------------|--------------------------------------|-------|
| Select section   | `FF E0 00 S 00 00 00`                | S: 0 = spectrum, 1 = ambient, 2 = effect |
| Set mode         | `FF E0 M idx 00 00 00`               | M: 1 = spectrum mode, 4 = ambient mode, 6 = effect mode; `idx` = mode number within section |
| Slider           | `FF E0 C hueHi hueLo bri type`       | C: 2 (spectrum), 5 (ambient); hue 0–360 big-endian 16-bit, brightness 0–200, type 1 = colour changed, 2 = brightness changed |
| Power            | `FF E0 FF <0|1> 00 00 00`            | |

Group 0xE0 is a good sign that group numbers are product-specific, i.e. the
clock firmware will most likely ignore 0xE0 and the lamp will ignore 0x01/0x04/0x05
**[guess]**.

---

## 6. Legacy nixie protocol (`XGGF-IN12xx / IN14xx`) — not clock, for reference

Completely different transport **[verified in code]**: HM-10-style modules,
services `ffe0` (read/notify char `ffe4`), `ffe5` (write char `ffe9`),
`fff0` (GPIO: `fff1` state, `fff2` set, `fff3` notify), `ffc0` (safety key:
write `ffc1`, notify `ffc2`). On connect the app writes an ASCII
"anti-hijack" key (default `210709210709`) to `ffc1`, then `OSC` to `ffe9` to
request a 93-character ASCII state string. Commands are ASCII, e.g.
`AHH:MMA` / `AHH:MMB` (alarm 1/2), `THH:MMO` / `THH:MMC` (power-on/off time),
`NAME:…`, `ID:…`, `S1`… (scene). None of this applies to the 1V48.

---

## 7. Practical notes for the CLI

- **Scan filter**: name starts with `XGGF`; expect exactly `XGGF-1V48`.
- **macOS caveat**: CoreBluetooth (used by both bleak and Go's
  tinygo.org/x/bluetooth) exposes a per-Mac random UUID as the "address",
  not the MAC. Store whatever `scan` prints; it is stable on the same Mac.
- **Minimum viable time sync**: connect, write `FF 01 00 YY MM DD 00 cs`,
  wait ~100 ms, write `FF 01 01 hh mm ss 00 cs`, disconnect. Use write-with-
  response, matching the app.
- To be safe against a mid-minute race, send the time packet just after a
  second boundary (sleep until `microsecond` is small) **[guess]**.
- If nothing works: capture an HCI snoop log from Android running WeChat
  (Developer options → "Enable Bluetooth HCI snoop log", reproduce, then
  `adb bugreport` and extract `btsnoop_hci.log`, open in Wireshark, filter
  `btatt.opcode == 0x12`) and compare the writes with §4.
