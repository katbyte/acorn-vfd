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
