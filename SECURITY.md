# Security Policy

## Supported Versions

Only the [latest release](https://github.com/katbyte/acorn-vfd/releases/latest) is supported — please update before reporting an issue.

## Reporting a Vulnerability

Please **do not** open a public issue for security vulnerabilities.

Instead, report privately via [GitHub's private vulnerability reporting](https://github.com/katbyte/acorn-vfd/security/advisories/new).

I will do my best to acknowledge reports within 2 weeks and aim to release a fix or mitigation within 6 weeks for confirmed issues; timelines are best-effort.

## Scope

`acornvfd` connects to Bluetooth LE devices and writes command packets built from user input. Relevant issues include packet construction that could send unintended writes to a device, and anything in the release pipeline (the tap-publishing token) that could leak a credential. The tool itself stores no user credentials; it only remembers a device address locally.
