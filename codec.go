package libpack

import (
	"fmt"
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
