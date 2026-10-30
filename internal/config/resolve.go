package config

import (
	"fmt"
	"maps"
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

// EnvLayer reads the environment layer: WT_<KEY> for every registered key
// but those marked NoEnv. A variable set to the empty string counts as
// unset, and WT_ variables that match no key are not read at all.
func EnvLayer(reg Registry, getenv func(string) string) Layer {
	values := map[string]any{}
	for _, k := range reg {
		if k.NoEnv {
			continue
		}
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
// A key whose name has a . is written in a file as a key of the table named
// by the part before the .: hooks.post-create is post-create in [hooks].
// Such a table is read as its keys, so an unknown key of it is named
// hooks.<key> in its warning.
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
		if layer.Source != SourceEnv {
			flat, err := flattenTables(reg, layer)
			if err != nil {
				return nil, err
			}
			layer.Values = flat
		}
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

// flattenTables replaces each table of a file layer whose name the registry
// uses as a prefix, such as [hooks], by its keys: post-create in [hooks]
// becomes hooks.post-create. Any other value under that name is an error.
// The layer's own map is not changed.
func flattenTables(reg Registry, layer Layer) (map[string]any, error) {
	values := maps.Clone(layer.Values)
	for _, table := range tables(reg) {
		v, ok := values[table]
		if !ok {
			continue
		}
		t, ok := v.(map[string]any)
		if !ok {
			example, _ := strings.CutPrefix(reg.firstIn(table), table+".")
			return nil, fmt.Errorf("invalid value for %q in %s: expected a table, got %s; write it as [%s] with keys such as %s",
				table, layer.Origin, describe(v), table, example)
		}
		delete(values, table)
		for k, kv := range t {
			values[table+"."+k] = kv
		}
	}
	return values, nil
}

// tables are the names before the . of the registry's keys, in order.
func tables(reg Registry) []string {
	var names []string
	for _, k := range reg {
		if table, _, ok := strings.Cut(k.Name, "."); ok && !slices.Contains(names, table) {
			names = append(names, table)
		}
	}
	return names
}

// firstIn is the first key of the table, as hooks.pre-create.
func (r Registry) firstIn(table string) string {
	for _, k := range r {
		if strings.HasPrefix(k.Name, table+".") {
			return k.Name
		}
	}
	return ""
}
