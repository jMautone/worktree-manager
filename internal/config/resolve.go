package config

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Source names the layer an effective value came from.
type Source string

// The layers, from lowest to highest precedence.
const (
	SourceDefault Source = "default"
	SourceUser    Source = "user"
	SourceRepo    Source = "repo"
	SourceEnv     Source = "env"
)

var precedence = []Source{SourceDefault, SourceUser, SourceRepo, SourceEnv}

// Layer is one source of values, already read and decoded.
type Layer struct {
	Source Source
	Origin string // the file the values came from; unused for SourceEnv
	Values map[string]any
}

// Value is the effective value of one key.
type Value struct {
	Key    string
	Value  any
	Source Source
	Origin string // file path or environment variable; empty for defaults
}

// Config is the resolved configuration.
type Config struct {
	Values   []Value // in registry order
	Warnings []string
}

// Get returns the effective value of key.
func (c *Config) Get(key string) (Value, bool) {
	for _, v := range c.Values {
		if v.Key == key {
			return v, true
		}
	}
	return Value{}, false
}

// EnvVar is the environment variable that sets key: WT_ and the key in upper
// case.
func EnvVar(key string) string {
	return "WT_" + strings.ToUpper(key)
}

// EnvLayer reads the environment layer: WT_<KEY> for every registered key. A
// variable set to the empty string counts as unset, and WT_ variables that
// match no key are not read at all.
func EnvLayer(reg Registry, getenv func(string) string) Layer {
	values := map[string]any{}
	for _, k := range reg {
		if v := getenv(EnvVar(k.Name)); v != "" {
			values[k.Name] = v
		}
	}
	return Layer{Source: SourceEnv, Values: values}
}

// Resolve computes the effective configuration. Precedence comes from each
// layer's Source (default < user < repo < env), not from argument order.
//
// A value from the environment goes through the key's FromEnv, when it has
// one, before Validate: WT_CREATE_CD=0 is a boolean, while create_cd = "0" in
// a file is a string, and an error.
//
// An unknown key in a file, and a key the repository file may not set, are
// ignored with a warning. A value of the wrong type fails with an error naming
// the key and where it was set, even when a higher layer overrides it: a
// broken file should be fixed, not silently shadowed.
func Resolve(reg Registry, layers ...Layer) (*Config, error) {
	layers = slices.Clone(layers)
	sort.SliceStable(layers, func(i, j int) bool {
		return slices.Index(precedence, layers[i].Source) < slices.Index(precedence, layers[j].Source)
	})

	cfg := &Config{}
	for _, k := range reg {
		cfg.Values = append(cfg.Values, Value{Key: k.Name, Value: k.Default, Source: SourceDefault})
	}
	for _, layer := range layers {
		names := make([]string, 0, len(layer.Values))
		for name := range layer.Values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			origin := layer.Origin
			if layer.Source == SourceEnv {
				origin = EnvVar(name)
			}
			k, ok := reg.Lookup(name)
			if !ok {
				cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("unknown key %q in %s", name, origin))
				continue
			}
			if layer.Source == SourceRepo && !k.InRepo {
				cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("ignoring %q in %s: repository configuration may not set it", name, origin))
				continue
			}
			raw := layer.Values[name]
			var err error
			if s, ok := raw.(string); ok && layer.Source == SourceEnv && k.FromEnv != nil {
				raw, err = k.FromEnv(s)
			}
			v := raw
			if err == nil {
				v, err = k.Validate(raw)
			}
			if err != nil {
				return nil, fmt.Errorf("invalid value for %q in %s: %v", name, origin, err)
			}
			i := slices.IndexFunc(cfg.Values, func(v Value) bool { return v.Key == name })
			cfg.Values[i] = Value{Key: name, Value: v, Source: layer.Source, Origin: origin}
		}
	}
	return cfg, nil
}
