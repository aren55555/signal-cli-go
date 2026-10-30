# Security policy

signal-cli-go handles end-to-end encryption keys and implements Signal's
cryptographic protocols itself (PQXDH, the Double Ratchet with SPQR, sealed
sender, sender keys, zkgroup, enclave attestation). Treat it as experimental. It
has not been audited, and it currently works against Signal's staging servers
only.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's private vulnerability
reporting: open the repository's **Security** tab and choose **Report a
vulnerability**. Don't open a public issue for a security problem.

Include what you found, the affected component or file, how to reproduce it (on
staging or offline), and what an attacker could do. If you have a fix, attach it
to the report rather than opening a public pull request.

The aim is to acknowledge reports within a week. The fix and its disclosure
are coordinated with you. There is no bug bounty.

## Scope

In scope:

- the cryptography and protocol implementations under `internal/` (key handling,
  constant-time behaviour, message parsing of untrusted input, attestation);
- the local store (file permissions, crash safety, key material on disk) and the
  importer;
- the JSON-RPC daemon (for example request handling on the HTTP, socket and TCP
  transports).

Out of scope, and better reported to their own projects:

- vulnerabilities in Signal's servers or official clients: report to Signal;
- vulnerabilities in signal-cli itself: report to signal-cli;
- issues in the vendored third-party modules, unless signal-cli-go's use of
  them causes the issue.

Behaviour that only differs from signal-cli 0.14.7 without a security impact is
a normal bug: please open an issue.

## Supported versions

Only the latest commit on the default branch is supported. There are no
releases yet.
