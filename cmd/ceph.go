package cmd

import (
	"fmt"
	"sort"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
)

func connectCeph() (*rados.Conn, error) {
	if err := validateConfig(); err != nil {
		return nil, err
	}

	conn, err := rados.NewConnWithUser(cephUser)
	if err != nil {
		return nil, fmt.Errorf("rados.NewConnWithUser: %w", err)
	}

	if monitors != "" {
		if err := conn.SetConfigOption("mon_host", monitors); err != nil {
			conn.Shutdown()
			return nil, fmt.Errorf("set mon_host: %w", err)
		}
	}

	if key != "" {
		if err := conn.SetConfigOption("key", key); err != nil {
			conn.Shutdown()
			return nil, fmt.Errorf("set key: %w", err)
		}
	} else if keyfile != "" {
		if err := conn.SetConfigOption("keyring", keyfile); err != nil {
			conn.Shutdown()
			return nil, fmt.Errorf("set keyring: %w", err)
		}
	}

	if err := conn.SetConfigOption("log_to_stderr", "false"); err != nil {
		conn.Shutdown()
		return nil, fmt.Errorf("set log_to_stderr: %w", err)
	}

	// Only fall back to ceph.conf if monitors or key are not provided.
	if monitors == "" || (key == "" && keyfile == "") {
		if err := conn.ReadDefaultConfigFile(); err != nil {
			conn.Shutdown()
			return nil, fmt.Errorf("no monitors/key provided and ceph.conf not found: %w", err)
		}
	}

	if err := conn.Connect(); err != nil {
		conn.Shutdown()
		return nil, fmt.Errorf("rados connect: %w", err)
	}

	return conn, nil
}

type imageInfo struct {
	Pool string
	Name string
	Size uint64
	Err  error
}

// listAllImages lists RBD images across all configured pools.
func listAllImages(conn *rados.Conn) ([]imageInfo, error) {
	var all []imageInfo
	for _, p := range pools {
		ioctx, err := conn.OpenIOContext(p)
		if err != nil {
			return nil, fmt.Errorf("open pool %q: %w", p, err)
		}

		names, err := rbd.GetImageNames(ioctx)
		if err != nil {
			ioctx.Destroy()
			return nil, fmt.Errorf("list images in %q: %w", p, err)
		}
		sort.Strings(names)

		for _, name := range names {
			info := imageInfo{Pool: p, Name: name}

			img, err := rbd.OpenImageReadOnly(ioctx, name, rbd.NoSnapshot)
			if err != nil {
				info.Err = err
				all = append(all, info)
				continue
			}
			info.Size, _ = img.GetSize()
			img.Close()
			all = append(all, info)
		}
		ioctx.Destroy()
	}
	return all, nil
}

// openPoolMap opens an IOContext for each configured pool.
func openPoolMap(conn *rados.Conn) (map[string]*rados.IOContext, error) {
	m := make(map[string]*rados.IOContext)
	for _, p := range pools {
		ioctx, err := conn.OpenIOContext(p)
		if err != nil {
			for _, existing := range m {
				existing.Destroy()
			}
			return nil, fmt.Errorf("open pool %q: %w", p, err)
		}
		m[p] = ioctx
	}
	return m, nil
}

func humanSize(bytes uint64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
		tib = gib * 1024
	)
	switch {
	case bytes >= tib:
		return fmt.Sprintf("%.1f TiB", float64(bytes)/float64(tib))
	case bytes >= gib:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/float64(gib))
	case bytes >= mib:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/float64(mib))
	case bytes >= kib:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/float64(kib))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
