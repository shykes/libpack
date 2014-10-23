package libpack

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	git "github.com/libgit2/git2go"
)

func getAnnotation(db *DB, name string) (string, error) {
	return db.Get(MkAnnotation(name))
}

func setAnnotation(db *DB, name, value string) error {
	return db.Set(MkAnnotation(name), value)
}

func walkAnnotations(db *DB, h func(name, value string)) error {
	return db.Walk("/", func(k string, obj git.Object) error {
		blob, isBlob := obj.(*git.Blob)
		if !isBlob {
			return nil
		}
		targetPath, err := ParseAnnotation(k)
		if err != nil {
			return err
		}
		h(targetPath, string(blob.Contents()))
		return nil
	})
}

func MkAnnotation(target string) string {
	target = TreePath(target)
	if target == "/" {
		return "0"
	}
	return fmt.Sprintf("%d/%s", strings.Count(target, "/")+1, target)
}

func ParseAnnotation(annot string) (target string, err error) {
	annot = TreePath(annot)
	parts := strings.Split(annot, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid annotation path")
	}
	lvl, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return "", err
	}
	if len(parts)-1 != int(lvl) {
		return "", fmt.Errorf("invalid annotation path")
	}
	return path.Join(parts[1:]...), nil
}
