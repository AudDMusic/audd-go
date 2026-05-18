# Security Policy

## Reporting a vulnerability

If you discover a security issue in this SDK, please email **api@audd.io** privately. Do not open a public GitHub issue for security reports.

We will acknowledge receipt within 2 business days and coordinate disclosure with you.

## Scope

- **In scope:** vulnerabilities in this SDK's source code.
- **Out of scope:** issues in upstream dependencies (file those with the upstream maintainer), or issues in the AudD service or API itself (email **api@audd.io** with subject `AudD service: <summary>`).

## Hardening practices

This SDK never logs `api_token`, request bodies, or response bodies. The `onEvent` inspection hook receives request / response / exception lifecycle events with method, URL, HTTP status, elapsed time, and request_id — but never the token or payload bytes.

