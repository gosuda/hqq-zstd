# hqq-zstd (`gosuda.org/hqq-zstd`)

`gosuda.org/hqq-zstd` is a **zero-`cgo` (`CGO_ENABLED=0`) Zstandard (`libzstd`) library for Go** powered by HQQ shared-memory IPC. Your Go binary compiles without linking C objects, while executing the official C `libzstd` (`1.5.7`) compressor and decompressor in an isolated shared-memory worker process (`zstd_server`).

```bash
go get gosuda.org/hqq-zstd
```

## Supported Platforms

| OS | Architectures | Prebuilt `zstd_server` Release Asset |
| :--- | :--- | :--- |
| **Linux** | `amd64`, `arm64` | `zstd_server_linux_amd64`, `zstd_server_linux_arm64` |
| **macOS** | `arm64` (Apple Silicon), `amd64` | `zstd_server_darwin_arm64`, `zstd_server_darwin_amd64` |
| **Windows** | `amd64`, `arm64` | `zstd_server_windows_amd64.exe` (`arm64` via source build) |

---

## Quickstart

### 1. One-Shot Compression and Decompression

Start `zstd_server` via `gosuda.org/hqq/launch` and connect with `zstd.Dial`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"gosuda.org/hqq-zstd"
	"gosuda.org/hqq/launch"
)

func main() {
	ctx := context.Background()

	hash, ok := zstd.ExpectedHash(runtime.GOOS, runtime.GOARCH)
	if !ok {
		log.Fatalf("no pinned digest for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	srv, err := launch.Start(ctx, launch.Config{
		Binary: "/opt/hqq/bin/zstd_server",
		SHA256: hash,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	client, err := zstd.Dial(ctx, srv.Listener(), zstd.DefaultOptions())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	src := []byte("zero-cgo Zstandard over HQQ shared memory")
	dst := make([]byte, zstd.CompressBound(uint64(len(src))))

	compressed, err := client.Compress(dst, src, 3)
	if err != nil {
		log.Fatal(err)
	}

	restored, err := client.Decompress(make([]byte, len(src)), compressed)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d B -> %d B -> %q\n", len(src), len(compressed), restored)
}
```

### 2. Pipelined Batch Calls (`*Future`)

Dispatch multiple blocks concurrently over the shared-memory ring with `CompressAsync` and `DecompressAsync`:

```go
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

### Session & Pool (`package zstd`)

```go
const (
	Version           = "v0.1.10507"
	ZstdVersionNumber = 10507
)

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
