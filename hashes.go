package zstd

import "gosuda.org/hqq/secureexec"

// Version follows the v<WrapperMajor>.<WrapperMinor>.<NormalizedCVersion> scheme,
// where 10507 corresponds to ZSTD_VERSION_NUMBER (Zstandard 1.5.7).
const (
	Version           = "v0.1.10507"
	ZstdVersionNumber = 10507
)

// ReleaseHashes maps "<GOOS>/<GOARCH>" to the SHA-256 digest of the prebuilt
// zstd_server binary published with this module version. The release workflow
// populates these entries alongside SLSA build provenance attestations.
var ReleaseHashes = map[string]secureexec.Hash{}

// ExpectedHash returns the pinned SHA-256 digest of zstd_server for goos/goarch.
func ExpectedHash(goos, goarch string) (secureexec.Hash, bool) {
	h, ok := ReleaseHashes[goos+"/"+goarch]
	return h, ok
}

func mustHash(s string) secureexec.Hash {
	h, err := secureexec.ParseHash(s)
	if err != nil {
		panic(err)
	}
	return h
}
