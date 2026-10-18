# hqq-zstd (`gosuda.org/hqq-zstd`)

`gosuda.org/hqq-zstd` is a **zero-`cgo` (`CGO_ENABLED=0`) Zstandard (`libzstd`) library for Go** powered by HQQ shared-memory IPC. Your Go binary compiles without linking C objects, while executing the official C `libzstd` (`1.5.7`) compressor and decompressor in an isolated shared-memory worker process (`zstd_server`).

SLSA-attested prebuilt `zstd_server` binaries are embedded per target platform (`//go:build` + `//go:embed`), extracted to the user cache on first use, verified against pinned SHA-256 digests via `gosuda.org/hqq/secureexec`, and bound to the Go parent process lifetime via `--parent-stdin`. No external binary download or C toolchain is required.

```bash
go get gosuda.org/hqq-zstd
```

## Supported Platforms

| OS | Architectures | Embedded & SLSA-Attested Release Binary |
| :--- | :--- | :--- |
| **Linux** | `amd64`, `arm64` | `zstd_server_linux_amd64`, `zstd_server_linux_arm64` (embedded) |
| **macOS** | `arm64` (Apple Silicon), `amd64` | `zstd_server_darwin_arm64`, `zstd_server_darwin_amd64` (embedded) |
| **Windows** | `amd64`, `arm64` | `zstd_server_windows_amd64.exe`, `zstd_server_windows_arm64.exe` (embedded) |

---

## Quickstart

### 1. Zero-Config Compression and Decompression

Call top-level `zstd.Compress` and `zstd.Decompress` (or `zstd.Open(ctx)` for a dedicated session) immediately after importing `gosuda.org/hqq-zstd`:

```go
package main

import (
	"fmt"
	"log"

	"gosuda.org/hqq-zstd"
)

func main() {
	src := []byte("zero-cgo Zstandard over HQQ shared memory")

	compressed, err := zstd.Compress(nil, src, 3)
	if err != nil {
		log.Fatal(err)
	}

	restored, err := zstd.Decompress(nil, compressed)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d B -> %d B -> %q\n", len(src), len(compressed), restored)
}
```

### 2. Dedicated Session & Pipelined Batch Calls (`zstd.Open`, `*Future`)

Open a dedicated shared-memory session via `zstd.Open(ctx)` to dispatch multiple blocks concurrently over the shared-memory ring with `CompressAsync` and `DecompressAsync`:

```go
client, err := zstd.Open(ctx)
if err != nil {
	return err
}
defer client.Close()

futures := make([]zstd.CompressFuture, len(blocks))
for i, block := range blocks {
	dst := make([]byte, zstd.CompressBound(uint64(len(block))))
	fut, err := client.CompressAsync(dst, block, 1)
	if err != nil {
		return err
	}
	futures[i] = fut
}

for i, fut := range futures {
	frame, err := fut.Wait()
	if err != nil {
		return err
	}
	_ = frame // compressed output for blocks[i]
}
```

### 3. Reusable Contexts & Streaming (`CCtx`, `DCtx`, `CompressStream2Flat`)

```go
cctx, err := client.CreateCCtx()
if err != nil {
	return err
}
defer client.FreeCCtx(cctx)

if _, err := client.CCtxSetParameter(cctx, zstd.ZSTD_c_compressionLevel, 3); err != nil {
	return err
}
if _, err := client.CCtxSetParameter(cctx, zstd.ZSTD_c_checksumFlag, 1); err != nil {
	return err
}

// Stream chunks with ZSTD_e_continue, finishing with ZSTD_e_end:
rem, out, consumed, err := client.CompressStream2Flat(cctx, dstBuf, chunk, zstd.ZSTD_e_end)
```

---

## Public API Summary

### Top-Level Zero-Config & Session Management (`package zstd`)

```go
const (
	Version           = "v0.1.10507"
	ZstdVersionNumber = 10507
)

// Zero-config one-shot helpers (auto-starts embedded zstd_server on first call)
func Compress(dst, src []byte, compressionLevel int32) ([]byte, error)
func Decompress(dst, src []byte) ([]byte, error)

// Auto-launched client sessions and managed server helpers
func Open(ctx context.Context) (*Client, error)
func OpenWith(ctx context.Context, opts rt.Options) (*Client, error)
func StartServer(ctx context.Context) (*launch.Server, error)
func ServerBinary() (path string, hash secureexec.Hash, err error)

// Explicit listener dialing & pooling
func DefaultOptions() rt.Options
func ExpectedHash(goos, goarch string) (secureexec.Hash, bool)
func CompressBound(srcSize uint64) uint64
func Dial(ctx context.Context, name string, opts rt.Options) (*Client, error)
func NewPool(name string, opts rt.Options, max int) *rt.Pool[*Client]
func (c *Client) Close() error
```

### Compression & Decompression (`*zstd.Client`)

```go
// One-shot
func (c *Client) Compress(dst []byte, src []byte, compressionLevel int32) ([]byte, error)
func (c *Client) CompressAsync(dst []byte, src []byte, compressionLevel int32) (CompressFuture, error)
func (c *Client) Decompress(dst []byte, src []byte) ([]byte, error)
func (c *Client) DecompressAsync(dst []byte, src []byte) (DecompressFuture, error)
func (c *Client) GetFrameContentSize(src []byte) (uint64, error)

// Reusable contexts
func (c *Client) CreateCCtx() (CCtx, error)
func (c *Client) FreeCCtx(cctx CCtx) error
func (c *Client) CompressCCtx(cctx CCtx, dst []byte, src []byte, compressionLevel int32) ([]byte, error)
func (c *Client) CCtxSetParameter(cctx CCtx, param uint32, value int32) (uint64, error)
func (c *Client) CCtxSetPledgedSrcSize(cctx CCtx, pledgedSrcSize uint64) (uint64, error)
func (c *Client) CCtxReset(cctx CCtx, reset uint32) (uint64, error)

func (c *Client) CreateDCtx() (DCtx, error)
func (c *Client) FreeDCtx(dctx DCtx) error
func (c *Client) DecompressDCtx(dctx DCtx, dst []byte, src []byte) ([]byte, error)
func (c *Client) DCtxReset(dctx DCtx, reset uint32) (uint64, error)

// Streaming
func (c *Client) CompressStream2Flat(cctx CCtx, dst []byte, src []byte, endOp uint32) (rem uint64, out []byte, consumed uint64, err error)
func (c *Client) CompressStream2FlatAsync(cctx CCtx, dst []byte, src []byte, endOp uint32) (CompressStream2FlatFuture, error)
func (c *Client) DecompressStreamFlat(dctx DCtx, dst []byte, src []byte) (rem uint64, out []byte, consumed uint64, err error)
func (c *Client) DecompressStreamFlatAsync(dctx DCtx, dst []byte, src []byte) (DecompressStreamFlatFuture, error)
```

### Error Handling (`package zstd`)

```go
type ErrorCode int

const (
	ErrPrefixUnknown        ErrorCode = 10
	ErrCorruptionDetected   ErrorCode = 20
	ErrChecksumWrong        ErrorCode = 22
	ErrParameterUnsupported ErrorCode = 40
	ErrParameterOutOfBound  ErrorCode = 42
	ErrStageWrong           ErrorCode = 60
	ErrDstSizeTooSmall      ErrorCode = 70
	ErrSrcSizeWrong         ErrorCode = 72
)

func ErrorCodeOf(err error) (ErrorCode, bool)
```

---

## Documentation and License

- [**`docs/ARCHITECTURE.md`**](docs/ARCHITECTURE.md): Shared-memory ring geometry, credit flow control, exposed IDL surface, security policy, and `cgo` comparison benchmarks.
- **License**: Wrapper code is licensed under the [MIT License](LICENSE); Zstandard (`1.5.7`) is used under the [BSD-3-Clause License](third_party/zstd/LICENSE).
