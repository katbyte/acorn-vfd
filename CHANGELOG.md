## Unreleased

- Fix a data race in scanning: results are now handed to the calling goroutine instead of being touched from the Bluetooth library's callback (which on macOS can still fire after the scan stops).
- `scan` lists a device again when its name turns up in a later packet than its address, instead of showing it as `(no name)` forever; `time` and friends find an intermittently-named clock the same way.
- `dump` and `listen` remember the clock's address like `time` does, and `dump` retries service discovery the way the app does instead of reporting `0 services` on a slow first answer.
- `lamp` commands no longer overwrite the remembered clock address, so a bare `acornvfd time` after a lamp command still talks to the clock.
- An unreadable state file is reported with `-v` and replaced on the next connection instead of silently disabling address reuse.
- `time` sends the time packet on a second boundary, so the clock's seconds land within Bluetooth latency of the computer's instead of anywhere in the second.
- `listen --duration 0` listens until Ctrl-C; `listen --dry-run` prints its `--send` packets without connecting.
- 7-byte `raw` / `--send` packets must start with `FF` (use `--no-checksum` to send anything else), matching the 8-byte check.
- Errors print as a plain `acornvfd: ...` line and honour `--silent`, instead of a timestamped log line.
- Device names and other untrusted text are no longer parsed for colour tags.

## v0.1.2 (2026-09-08)

- Sign releases: cosign keyless signature on `checksums.txt` (`checksums.txt.sigstore.json`), GitHub build-provenance attestation, and a SLSA provenance job, matching the other katbyte CLIs.
- Ship more targets: linux 386/arm(v6,v7)/riscv64 and windows/386 alongside the existing amd64/arm64 builds (everything the BLE library can build for).
- Bring CI and tooling into line with katbyte/tctest verbatim: workflows, golangci-lint config (formatters, full rule set), makefile.

## v0.1.1 (2026-09-07)

- Fix Homebrew formula publishing so a release updates the `katbyte/homebrew-tap` formula (`brew install katbyte/tap/acornvfd`).
- Tooling parity with the other katbyte CLIs: azproviderlint (AZG) via a custom golangci-lint build, plus CodeQL, govulncheck, and shellcheck workflows.
- Add SECURITY.md, CODE_OF_CONDUCT.md, and a photo of the clock in the README.

## v0.1.0 (2026-09-07)

- Initial release. Control the 橡果工坊 (Acorn Workshop) XGGF-1V48 VFD clock over Bluetooth LE from macOS, Linux, or Windows: `time`, `date`, `brightness`, `alarm`, `display`, `tick`, `transition`, `auto-sync`, plus `scan`, `dump`, `listen`, `raw`, and decoded-but-untested `lamp` commands. Protocol reverse-engineered from the vendor app; see PROTOCOL.md.
