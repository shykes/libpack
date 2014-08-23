package libpack

import (
	"testing"
)

func TestPackageDecode(t *testing.T) {
	tests := []struct {
		Path  string
		Data  string
		Valid bool
	}{
		{
			"shykes/myapp/0.8",
			`{
				"name": "shykes/myapp",
				"tag": "0.8",
				"description": "Just another app",
				"superfluous": "this field is not defined. It should be ignored",
				"commands": [
					["nop"],
					["echo", "installing..."],
					["unpack", "424242"]
				]
			}`,
			true,
		},
		{
			"shykes/myapp/0.8",
			`{
				"name": "shykes/myapp",
				"tag": "0.42",
				"description": "Just another app",
				"superfluous": "this field is not defined. It should be ignored",
				"commands": [
					["nop"],
					["echo", "installing..."],
					["unpack", "424242"]
				]
			}`,
			false,
		},
	}
	for _, test := range tests {
		pkg, err := DecodePkg([]byte(test.Data), test.Path)
		if test.Valid {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			if err == nil {
				t.Fatal(err)
			}
		}
		if test.Valid && pkg.Path() != test.Path {
			t.Fatalf("%#v", pkg)
		}
	}
}
