package libpack

import (
	"fmt"
	"path"
)

// Decode reads the contents of the key "/" and attempts to decode
// it into `val`. It uses type introspection in the same way than the
// standard package `encoding/json`.
//  * A tree is treated as a struct/object
//  * A file is treated as a string, int, float, or date
//	depending on the destination type.
//  * If the destination type is an array, each line of the file is
//      used as an entry, or files named "0", "1", 2" etc. are used
//      as entries for as long as they are contiguous.
func (t *Tree) Decode(key string, val interface{}) error {
	// TODO
	return fmt.Errorf("not implemented")
}

func (t *Tree) Encode(key string, val interface{}) (*Tree, error) {
	// TODO
	return nil, fmt.Errorf("not implemented")
}

func (t *Tree) GetMap(key string) (map[string]string, error) {
	entries, err := t.List(key)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for _, k := range entries {
		v, err := t.Get(path.Join(key, k))
		if err == nil {
			m[k] = v
		} else {
			submap, err := t.GetMap(path.Join(key, k))
			if err != nil {
				return nil, err
			}
			for subk, subv := range submap {
				m[k+"/"+subk] = subv
			}
		}
	}
	return m, nil
}
