package zstd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

	"gosuda.org/hqq-zstd/internal/embedded"
	"gosuda.org/hqq/bindgen/rt"
	"gosuda.org/hqq/launch"
	"gosuda.org/hqq/secureexec"
)

// ServerBinary materializes the embedded prebuilt zstd_server binary for the
// current OS and architecture, verifies its SHA-256 digest against
// ExpectedHash(runtime.GOOS, runtime.GOARCH), and returns its filesystem path
// and verified digest.
//
// If no embedded binary is available for the current platform, ServerBinary
// falls back to the HQQ_ZSTD_SERVER environment variable when set.
func ServerBinary() (string, secureexec.Hash, error) {
	if expected, ok := ExpectedHash(runtime.GOOS, runtime.GOARCH); ok {
		path, err := embedded.Materialize(expected)
		if err == nil {
			return path, expected, nil
		}
	}
	if p := os.Getenv("HQQ_ZSTD_SERVER"); p != "" {
		h, _, err := secureexec.HashFile(p)
		if err != nil {
			return "", secureexec.Hash{}, fmt.Errorf("zstd: hash HQQ_ZSTD_SERVER (%s): %w", p, err)
		}
		return p, h, nil
	}
	return "", secureexec.Hash{}, fmt.Errorf("zstd: no embedded zstd_server binary for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// StartServer verifies the embedded zstd_server binary with secureexec and
// starts an isolated worker process bound to the parent process lifetime via
// --parent-stdin.
func StartServer(ctx context.Context) (*launch.Server, error) {
	binPath, hash, err := ServerBinary()
	if err != nil {
		return nil, err
	}
	listener, err := launch.NewListenerName("hqqzstd")
	if err != nil {
		return nil, err
	}
	return launch.Start(ctx, launch.Config{
		Binary:   binPath,
		SHA256:   hash,
		Listener: listener,
		Args:     []string{listener},
	})
}

var (
	sharedMu   sync.Mutex
	sharedSrv  *launch.Server
	sharedPool = rt.NewPool(16, func(ctx context.Context) (*Client, error) {
		return Open(ctx)
	}, func(c *Client) error {
		return c.Close()
	})
)

func ensureSharedServer(ctx context.Context) (string, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedSrv != nil {
		select {
		case <-sharedSrv.Done():
			sharedSrv = nil
		default:
			return sharedSrv.Listener(), nil
		}
	}
	srv, err := StartServer(ctx)
	if err != nil {
		return "", err
	}
	sharedSrv = srv
	return srv.Listener(), nil
}

// Open connects a new *Client session to the automatically managed embedded
// zstd_server worker process using DefaultOptions().
func Open(ctx context.Context) (*Client, error) {
	return OpenWith(ctx, DefaultOptions())
}

// OpenWith connects a new *Client session to the automatically managed embedded
// zstd_server worker process using opts.
func OpenWith(ctx context.Context, opts rt.Options) (*Client, error) {
	listener, err := ensureSharedServer(ctx)
	if err != nil {
		return nil, err
	}
	cl, err := Dial(ctx, listener, opts)
	if err != nil {
		sharedMu.Lock()
		if sharedSrv != nil && sharedSrv.Listener() == listener {
			select {
			case <-sharedSrv.Done():
				sharedSrv = nil
			default:
			}
		}
		sharedMu.Unlock()
		return nil, err
	}
	return cl, nil
}

// Compress compresses src at compressionLevel using a pooled session connected
// to the embedded zstd_server worker. When cap(dst) == 0, Compress allocates a
// destination buffer sized to CompressBound(uint64(len(src))).
func Compress(dst, src []byte, compressionLevel int32) ([]byte, error) {
	if cap(dst) == 0 {
		bound := CompressBound(uint64(len(src)))
		if bound == 0 {
			return nil, errors.New("zstd: input too large")
		}
		dst = make([]byte, bound)
	}
	cl, err := sharedPool.Get(context.Background())
	if err != nil {
		return nil, err
	}
	out, err := cl.Compress(dst, src, compressionLevel)
	if err != nil {
		var le *rt.LibError
		if errors.As(err, &le) {
			sharedPool.Put(cl)
		} else {
			sharedPool.Discard(cl)
		}
		return nil, err
	}
	sharedPool.Put(cl)
	return out, nil
}

// Decompress decompresses a single Zstandard frame from src into dst using a
// pooled session connected to the embedded zstd_server worker. When
// cap(dst) == 0, Decompress inspects the frame header via GetFrameContentSize
// and allocates the destination buffer automatically (up to 128 MiB).
func Decompress(dst, src []byte) ([]byte, error) {
	cl, err := sharedPool.Get(context.Background())
	if err != nil {
		return nil, err
	}
	release := func(err error) {
		if err == nil {
			sharedPool.Put(cl)
			return
		}
		var le *rt.LibError
		if errors.As(err, &le) {
			sharedPool.Put(cl)
		} else {
			sharedPool.Discard(cl)
		}
	}
	if cap(dst) == 0 {
		hdr := src
		if len(hdr) > int(ZSTD_FRAMEHEADER_MAX) {
			hdr = hdr[:ZSTD_FRAMEHEADER_MAX]
		}
		sz, err := cl.GetFrameContentSize(hdr)
		if err != nil {
			release(err)
			return nil, err
		}
		const maxAutoAlloc = 128 << 20
		if sz == ZSTD_CONTENTSIZE_UNKNOWN || sz == ZSTD_CONTENTSIZE_ERROR {
			sharedPool.Put(cl)
			return nil, errors.New("zstd: frame content size unknown or invalid; pass a preallocated dst slice or use streaming")
		}
		if sz > maxAutoAlloc {
			sharedPool.Put(cl)
			return nil, fmt.Errorf("zstd: frame content size %d exceeds automatic allocation limit (%d bytes)", sz, maxAutoAlloc)
		}
		dst = make([]byte, sz)
	}
	out, err := cl.Decompress(dst, src)
	release(err)
	return out, err
}
