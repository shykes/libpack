package libpack

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	git "github.com/libgit2/git2go"
)

type GlobalTree struct {
	db *DB
}

func (g *GlobalTree) Get(key string) (string, error) {
	return g.db.Scope("data").Get(key)
}

func (g *GlobalTree) List(key string) ([]string, error) {
	return g.db.Scope("data").List(key)
}

func (g *GlobalTree) Dump(w io.Writer) error {
	return g.db.Scope("data").Dump(w)
}

func InitGlobal(repo, ref string) (*GlobalTree, error) {
	db, err := Init(repo, ref)
	if err != nil {
		return nil, err
	}
	return &GlobalTree{db}, nil
}

func OpenGlobal(repo, ref string) (*GlobalTree, error) {
	db, err := Open(repo, ref)
	if err != nil {
		return nil, err
	}
	return &GlobalTree{db}, nil
}

func (g *GlobalTree) ListMounts() ([]*Mount, error) {
	var mounts []*Mount
	err := walkAnnotations(g.db.Scope("mounts"), func(target, data string) {
		fmt.Printf("found mount annotation for %s\n", target)
		mnt, err := parseMount(data)
		if err != nil {
			// Ignore invalid mount entries
			return
		}
		mounts = append(mounts, mnt)
	})
	if err != nil {
		return nil, err
	}
	return mounts, nil
}

func (g *GlobalTree) LoadMount(mnt *Mount) error {
	err := setAnnotation(g.db.Scope("mounts"), mnt.Dst, mnt.String())
	if err != nil {
		return err
	}
	return g.db.Commit(fmt.Sprintf("LoadMount %s %s", mnt.Dst, mnt.Src))
}

func (g *GlobalTree) Mount(dst string) error {
	mnt, err := g.getMount(dst)
	if err != nil {
		return err
	}
	if err = g.db.Scope("data").Add(mnt.Dst, mnt.Src); err != nil {
		return err
	}
	return g.db.Commit(fmt.Sprintf("mount %s", dst))
}

func (g *GlobalTree) getMount(dir string) (*Mount, error) {
	blob, err := g.db.Scope("mounts").Get(annotationPath(dir))
	if err != nil {
		return nil, err
	}
	return parseMount(blob)
}

type Mount struct {
	Dst string
	Src *git.Oid
}

type mountSchema struct {
	Dst string `json:"dst"`
	// FIXME: symbolic reference (mount a name instead of a hash)
	Src string `json:"src"`
	// FIXME: signature
}

func (mnt *Mount) String() string {
	s, _ := json.Marshal(&mountSchema{
		Src: mnt.Src.String(),
		Dst: mnt.Dst,
	})
	return string(s)
}

func parseMount(data string) (*Mount, error) {
	var s mountSchema
	err := json.Unmarshal([]byte(data), &s)
	if err != nil {
		return nil, err
	}
	dst := TreePath(s.Dst)
	src, err := git.NewOid(s.Src)
	if err != nil {
		return nil, err
	}
	return &Mount{
		Dst: TreePath(dst),
		Src: src,
	}, nil
}

func getAnnotation(db *DB, name string) (string, error) {
	return db.Get(annotationPath(name))
}

func setAnnotation(db *DB, name, value string) error {
	return db.Set(annotationPath(name), value)
}

func walkAnnotations(db *DB, h func(name, value string)) error {
	return db.Walk("/", func(k string, obj git.Object) error {
		blob, isBlob := obj.(*git.Blob)
		if !isBlob {
			return nil
		}
		targetPath, err := asAnnotation(k)
		if err != nil {
			return err
		}
		h(targetPath, string(blob.Contents()))
		return nil
	})
}

func annotationPath(name string) string {
	name = TreePath(name)
	if name == "/" {
		return "0"
	}
	return fmt.Sprintf("%d/%s", strings.Count(name, "/")+1, name)
}

func asAnnotation(name string) (string, error) {
	name = TreePath(name)
	parts := strings.Split(name, "/")
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
