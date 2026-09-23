# Security policy

## Supported versions

RapidGo is pre-alpha. Until the first release, security fixes are made on `main` only. After releases begin, the latest minor release will be supported.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use [GitHub private vulnerability reporting](https://github.com/hangxie/rapidgo/security/advisories/new) and include reproduction steps, impact, and any suggested mitigation.

RapidGo executes the user's Go toolchain and project code during build, test, and run operations. Opening an untrusted repository will therefore carry the same risks as running its Go commands directly. Do not include secrets, private source, or identifying paths in reports unless needed to reproduce the problem.

