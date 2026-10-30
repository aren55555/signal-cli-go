// SPDX-License-Identifier: GPL-3.0-or-later

//go:build race

package repocheck

// raceEnabled: the race detector slows the secret scan from seconds to
// more than ten minutes, and the scan has no concurrency to check; it runs
// without -race instead (`just secret-scan`, CI).
const raceEnabled = true
