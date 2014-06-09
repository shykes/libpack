package main

import (
	"log"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"github.com/dotcloud/docker/pkg/system"
	"github.com/dotcloud/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

func main() {
	target := os.Args[1]
	tw := tar.NewWriter(os.Stdout)
	filepath.Walk(target, func(filePath string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relFilePath, err := filepath.Rel(target, filePath)
		if err != nil {
			return err
		}
		if err := addTarFile(filePath, relFilePath, tw, false); err != nil {
			return err
		}
		return nil
	})
	/*
	if err := addTarFile(target, target, tw, false); err != nil {
		log.Fatal(err)
	}
	*/
	if err := tw.Close(); err != nil {
		log.Fatal(err)
	}
}

func addTarFile(path, name string, tw *tar.Writer, writeData bool) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}

	link := ""
	if fi.Mode()&os.ModeSymlink != 0 {
		if link, err = os.Readlink(path); err != nil {
			return err
		}
	}

	hdr, err := tar.FileInfoHeader(fi, link)
	if err != nil {
		return err
	}

	if fi.IsDir() && !strings.HasSuffix(name, "/") {
		name = name + "/"
	}

	hdr.Name = name

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		// Currently go does not fill in the major/minors
		if stat.Mode&syscall.S_IFBLK == syscall.S_IFBLK ||
			stat.Mode&syscall.S_IFCHR == syscall.S_IFCHR {
			hdr.Devmajor = int64(major(uint64(stat.Rdev)))
			hdr.Devminor = int64(minor(uint64(stat.Rdev)))
		}

	}

	capability, _ := system.Lgetxattr(path, "security.capability")
	if capability != nil {
		hdr.Xattrs = make(map[string]string)
		hdr.Xattrs["security.capability"] = string(capability)
	}

	if !writeData {
		hdr.Size = 0
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	if writeData {
		if hdr.Typeflag == tar.TypeReg {
			if file, err := os.Open(path); err != nil {
				return err
			} else {
				_, err := io.Copy(tw, file)
				if err != nil {
					return err
				}
				file.Close()
			}
		}
	}

	return nil
}

func major(device uint64) uint64 {
	return (device >> 8) & 0xfff
}

func minor(device uint64) uint64 {
	return (device & 0xff) | ((device >> 12) & 0xfff00)
}

