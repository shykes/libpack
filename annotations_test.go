package libpack

import (
	"testing"
)

func TestMkAnnotation(t *testing.T) {
	resultMap := map[string]string{
		"/":        "0",
		"/one":     "1/one",
		"/one/two": "2/one/two",
	}

	for original, annotation := range resultMap {
		if MkAnnotation(original) != annotation {
			t.Fatalf("Path %q does not correspond to annotation %q", original, annotation)
		}
	}
}

func TestParseAnnotation(t *testing.T) {
	resultMap := map[string]string{
		"0":         "",
		"1/one":     "one",
		"2/one/two": "one/two",
	}

	for annotation, original := range resultMap {
		if target, err := ParseAnnotation(annotation); err == nil {
			if target != original {
				t.Fatalf("Annotation %q does not equal original path %q: %q", annotation, original, target)
			}
		} else {
			t.Fatal(err)
		}
	}
}
