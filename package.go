package libpack

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

type Package struct {
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	Description string `json:"description,omitempty"`
	Commands    []Cmd  `json:"commands"`
	// FIXME: signature
}

type Cmd []string

func (pkg *Package) Path() string {
	return TreePath(path.Join(pkg.Name, pkg.Tag))
}

func (pkg *Package) String() string {
	j, err := json.Marshal(pkg)
	if err != nil {
		return ""
	}
	return string(j)
}
func (pkg *Package) Install(ins Installer) error {
	for _, cmd := range pkg.Commands {
		if len(cmd) == 0 {
			continue
		}
		op := strings.ToLower(cmd[0])
		args := cmd[1:]
		var err error
		switch op {
		case "nop":
			{
				if len(args) != 0 {
					return fmt.Errorf("usage: nop")
				}
				err = ins.Nop()
			}
		case "echo":
			{
				err = ins.Echo(strings.Join(args, " "))
			}
		case "unpack":
			{
				if len(args) != 2 {
					return fmt.Errorf("usage: unpack HASH DEST")
				}
				err = ins.Unpack(args[0], args[1])
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func DecodePkg(val []byte, path string) (*Package, error) {
	var pkg Package
	err := json.Unmarshal(val, &pkg)
	if err != nil {
		return nil, err
	}
	path = TreePath(path)
	if path != pkg.Path() {
		return nil, fmt.Errorf("Misplaced image: described as %s but stored at %s", pkg.Path(), path)
	}
	return &pkg, nil
}

type Installer interface {
	Nop() error
	Echo(string) error
	Unpack(string, string) error
}
