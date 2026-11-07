package cmdutil

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/pflag"
	"go.yaml.in/yaml/v3"
)

// Registerer is a config that declares its keys as flags.
type Registerer interface {
	RegisterFlags(fs *pflag.FlagSet)
}

// Load fills cfg from, lowest first: the flag defaults, the --config file, the
// environment, and the command line. It returns every error it finds at once.
func Load(name string, cfg Registerer, args []string, lookup func(string) (string, bool)) error {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetNormalizeFunc(underscoreIsDash)
	configPath := fs.String("config", "", "Path to a YAML config file.")
	cfg.RegisterFlags(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}

	file := map[string]string{}
	if *configPath != "" {
		var err error
		file, err = readFile(*configPath)
		if err != nil {
			return fmt.Errorf("read config file %s: %w", *configPath, err)
		}
	}

	var errs []error
	for _, key := range slices.Sorted(maps.Keys(file)) {
		if key == "config" || fs.Lookup(key) == nil {
			errs = append(errs, fmt.Errorf("%s: unknown key %s", *configPath, key))
		}
	}

	seen := map[string]string{}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name == "config" {
			return
		}

		env := EnvName(f.Name)
		if other, ok := seen[env]; ok {
			errs = append(errs, fmt.Errorf("flags %s and %s both map to %s", other, f.Name, env))
			return
		}
		seen[env] = f.Name

		// A pflag slice appends on its second Set, so each flag takes one value at most.
		if f.Changed {
			return
		}

		var value, source string
		if v, ok := lookup(env); ok {
			value, source = v, env
		} else if v, ok := file[f.Name]; ok {
			value, source = v, *configPath
		} else {
			return
		}

		if err := fs.Set(f.Name, value); err != nil {
			errs = append(errs, fmt.Errorf("%s: %s: %w", source, f.Name, err))
		}
	})

	return errors.Join(errs...)
}

// EnvName turns postgres.max-conns into POSTGRES_MAX_CONNS.
func EnvName(flag string) string {
	return strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(flag))
}

// FlagNames returns every flag that cfg registers, sorted.
func FlagNames(cfg Registerer) []string {
	fs := pflag.NewFlagSet("", pflag.ContinueOnError)
	cfg.RegisterFlags(fs)

	var names []string
	fs.VisitAll(func(f *pflag.Flag) {
		names = append(names, f.Name)
	})

	return names
}

func underscoreIsDash(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
}

// readFile flattens a YAML file into flag names: a nested key becomes a dotted
// name and a list becomes one comma-separated value.
func readFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}

	out := map[string]string{}
	if err := flatten("", doc, out); err != nil {
		return nil, err
	}

	return out, nil
}

func flatten(prefix string, in map[string]any, out map[string]string) error {
	for k, v := range in {
		key := strings.ReplaceAll(prefix+k, "_", "-")

		switch v := v.(type) {
		case map[string]any:
			if err := flatten(key+".", v, out); err != nil {
				return err
			}
		case []any:
			parts := make([]string, len(v))
			for i, item := range v {
				parts[i] = fmt.Sprint(item)
			}
			out[key] = strings.Join(parts, ",")
		case nil:
			return fmt.Errorf("%s has no value", key)
		default:
			out[key] = fmt.Sprint(v)
		}
	}

	return nil
}
