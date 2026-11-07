package configutil

import (
	"errors"
	"fmt"
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

	var errs []error

	var file map[string]string
	if *configPath != "" {
		var fileErrs []error
		file, fileErrs = readFile(*configPath, fs)
		errs = append(errs, fileErrs...)
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
// name and a list becomes one comma-separated value. It returns every problem
// with a key, sorted.
func readFile(path string, fs *pflag.FlagSet) (map[string]string, []error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, []error{fmt.Errorf("read config file: %w", err)}
	}

	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, []error{fmt.Errorf("read config file %s: %w", path, err)}
	}

	values := map[string]string{}
	sources := map[string]string{}
	var problems []string
	flatten("", "", doc, values, sources, &problems)

	for key, source := range sources {
		if key == "config" || fs.Lookup(key) == nil {
			problems = append(problems, "unknown key "+source)
		}
	}

	slices.Sort(problems)
	errs := make([]error, len(problems))
	for i, problem := range problems {
		errs[i] = fmt.Errorf("%s: %s", path, problem)
	}

	return values, errs
}

// flatten fills values by flag name, and sources with the key as the file
// spells it.
func flatten(prefix, rawPrefix string, in map[string]any, values, sources map[string]string, problems *[]string) {
	for k, v := range in {
		raw := rawPrefix + k
		key := strings.ReplaceAll(prefix+k, "_", "-")

		var value string
		switch v := v.(type) {
		case map[string]any:
			flatten(key+".", raw+".", v, values, sources, problems)
			continue
		case nil:
			*problems = append(*problems, raw+" has no value")
			continue
		case []any:
			parts := make([]string, len(v))
			for i, item := range v {
				parts[i] = fmt.Sprint(item)
			}
			value = strings.Join(parts, ",")
		default:
			value = fmt.Sprint(v)
		}

		if other, ok := sources[key]; ok {
			pair := []string{other, raw}
			slices.Sort(pair)
			*problems = append(*problems, fmt.Sprintf("%s and %s both set %s", pair[0], pair[1], key))
			continue
		}

		sources[key] = raw
		values[key] = value
	}
}
