# Security Policy

## Supported versions

Only the latest minor release receives fixes. The library has no dependencies
outside the Go standard library; keep your Go toolchain current to pick up
standard-library security fixes.

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/maslennikov-yv/pubsub/security/advisories/new).
Do not open a public issue. You should receive an acknowledgement within a
week; a fix or a mitigation plan follows as soon as the report is confirmed.

`Hash` uses MD5 as a non-cryptographic key fingerprint. It is not a security
primitive and must not be used as one.
