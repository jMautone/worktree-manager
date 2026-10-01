package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Load reads one configuration file as a layer. A missing file is not an
// error: it returns a nil layer. Values are decoded to a map rather than a
// struct so that Resolve sees unknown keys and wrong types and can report
// them with the key and the file.
func Load(path string, source Source) (*Layer, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading configuration: %w", err)
	}
	values := map[string]any{}
	if err := toml.Unmarshal(data, &values); err != nil {
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			row, col := derr.Position()
			return nil, fmt.Errorf("invalid TOML in %s at line %d, column %d: %s", path, row, col, strings.TrimPrefix(derr.Error(), "toml: "))
		}
		return nil, fmt.Errorf("invalid TOML in %s: %v", path, err)
	}
	return &Layer{Source: source, Origin: path, Values: values}, nil
}
