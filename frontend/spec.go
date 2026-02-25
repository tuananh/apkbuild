package frontend

import "github.com/tuananh/apkbuild/pkg/spec"

// LoadSpec parses YAML bytes into Spec.
func LoadSpec(data []byte) (*spec.Spec, error) {
	return spec.Load(data)
}
