package zstd

import "gosuda.org/hqq/secureexec"

// Version follows the v<WrapperMajor>.<WrapperMinor>.<NormalizedCVersion> scheme,
// where 10507 corresponds to ZSTD_VERSION_NUMBER (Zstandard 1.5.7).
const (
	Version           = "v0.1.10507"
	ZstdVersionNumber = 10507
)

// ReleaseHashes maps "<GOOS>/<GOARCH>" to the SHA-256 digest of the prebuilt
// zstd_server binary published with this module version.
var ReleaseHashes = map[string]secureexec.Hash{
	"darwin/amd64":  mustHash("f8e1b416ca7bb184c83800e30aa17e57cdefa464cf1267625b2923d925e2b6c0"),
	"darwin/arm64":  mustHash("63ef6166d9a27d19d3810b6b307fbd7d00fe204b4cca3b90f5c2f49b0bda430c"),
	"linux/amd64":   mustHash("c24140b14a8c513e2c918e742c964f47d328ed9c95fcb18d21249d0e0a4a6c47"),
	"linux/arm64":   mustHash("a9ecf66c244bb94858d3ba2c5f8c86525231ee6f4070215f4f364c3fd3c46473"),
	"windows/amd64": mustHash("79ea7be4a397cc9cc699980170707ce043191b6f02bc0e36b34bf36a077d69a8"),
	"windows/arm64": mustHash("a5b94878d208849934d739c5bb5eaa64a98864e143df6ca6cac0e34f06cfb428"),
}

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
