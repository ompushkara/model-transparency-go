// Copyright 2025 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package options

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

// stdinJSONArg reads JSON (or key=value text) from stdin when --json is this value.
const stdinJSONArg = "-"

// ErrUnknownJSONFlagKey is returned when --json contains a key that is not a defined
// (non-hidden) flag for the command being run.
var ErrUnknownJSONFlagKey = errors.New("unknown JSON key: not a defined flag for this command")

// JSONFlags defines --json: set other flags from a JSON object or key=value pairs (repeat to merge).
type JSONFlags struct {
	jsonInputs []string
}

// NewJSONFlags returns an empty JSONFlags. Register other flags on the command first,
// then AddPersistentFlags, then call ParseAndApply(cmd) so allowed keys match existing flags.
func NewJSONFlags() *JSONFlags {
	return &JSONFlags{}
}

// AddPersistentFlags registers persistent --json (e.g. on the CLI root) so it is available on all subcommands.
func (o *JSONFlags) AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringArrayVar(&o.jsonInputs, "json", nil,
		fmt.Sprintf(`Set flags from JSON object and/or key=value (repeat to merge). Use %q to read JSON or key=value text from stdin. Keys must name this command's flags. Not the same as --log-format json. CLI flags override --json.`, stdinJSONArg))
}

// ParseAndApply merges --json into cmd flags when any --json value was set.
func (o *JSONFlags) ParseAndApply(cmd *cobra.Command) error {
	if !o.hasJSONInput() {
		return nil
	}
	data, err := o.parseWithStdin(cmd, os.Stdin)
	if err != nil {
		return err
	}
	return o.applyParsed(cmd, data)
}

func (o *JSONFlags) hasJSONInput() bool {
	for _, s := range o.jsonInputs {
		if strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

func (o *JSONFlags) applyParsed(cmd *cobra.Command, data map[string]string) error {
	for k, v := range data {
		f := cmd.Flag(k)
		if f == nil {
			return fmt.Errorf("internal error: flag %q not found after --json parse", k)
		}
		if !f.Changed {
			if err := f.Value.Set(v); err != nil {
				return fmt.Errorf("apply --json to flag %q: %w", k, err)
			}
			// Required-flag validation uses pflag.Changed; CLI did not set these.
			f.Changed = true
		}
	}
	return nil
}

// allowedFlagNames returns long names of flags available on cmd, excluding --json itself,
// help, and hidden flags.
func allowedFlagNames(cmd *cobra.Command) map[string]struct{} {
	names := make(map[string]struct{})
	visit := func(fs *flag.FlagSet) {
		fs.VisitAll(func(f *flag.Flag) {
			if f.Hidden {
				return
			}
			if f.Name == "json" || f.Name == helpFlagName {
				return
			}
			names[f.Name] = struct{}{}
		})
	}
	visit(cmd.Flags())
	visit(cmd.InheritedFlags())
	return names
}

const helpFlagName = "help"

func normalizeFlagKey(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return name
	}
	fn := cmd.Root().GlobalNormalizationFunc()
	if fn == nil {
		return name
	}
	return string(fn(cmd.Flags(), name))
}

// parseWithStdin merges and validates all --json values (stdin used when an argument is "-").
// For JSON objects, every key must be a defined (non-hidden) flag on cmd; unknown keys
// fail with an error wrapping ErrUnknownJSONFlagKey.
func (o *JSONFlags) parseWithStdin(cmd *cobra.Command, stdin io.Reader) (map[string]string, error) {
	allowed := allowedFlagNames(cmd)
	out := make(map[string]string)
	var stdinConsumed bool
	for _, rawIn := range o.jsonInputs {
		raw, err := materializeJSONArg(rawIn, stdin, &stdinConsumed)
		if err != nil {
			return nil, err
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "{") {
			if err := mergeJSONObject(cmd, out, allowed, raw); err != nil {
				return nil, err
			}
			continue
		}
		for _, part := range splitCommaSeparatedKV(raw) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if err := mergeKeyValue(cmd, out, allowed, part); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func materializeJSONArg(rawIn string, stdin io.Reader, stdinConsumed *bool) (string, error) {
	rawIn = strings.TrimSpace(rawIn)
	if rawIn != stdinJSONArg {
		return rawIn, nil
	}
	if *stdinConsumed {
		return "", fmt.Errorf("only one --json %s can read stdin", stdinJSONArg)
	}
	*stdinConsumed = true
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin for --json %s: %w", stdinJSONArg, err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", fmt.Errorf("stdin for --json %s was empty", stdinJSONArg)
	}
	return s, nil
}

func mergeJSONObject(cmd *cobra.Command, dst map[string]string, allowed map[string]struct{}, raw string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return fmt.Errorf("invalid JSON for --json: %w", err)
	}
	if obj == nil {
		return fmt.Errorf("invalid JSON for --json: must be a JSON object")
	}
	for key, rawMsg := range obj {
		nk := normalizeFlagKey(cmd, key)
		if _, ok := allowed[nk]; !ok {
			return fmt.Errorf("%w: %q", ErrUnknownJSONFlagKey, key)
		}
		s, err := stringifyJSONValue(rawMsg)
		if err != nil {
			return fmt.Errorf("key %q: %w", key, err)
		}
		dst[nk] = s
	}
	return nil
}

func stringifyJSONValue(raw json.RawMessage) (string, error) {
	raw = json.RawMessage(bytes.TrimSpace([]byte(raw)))
	if len(raw) == 0 {
		return "", nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	return stringifyInterface(v)
}

func stringifyInterface(v interface{}) (string, error) {
	switch v := v.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("unsupported JSON value type %T", v)
	}
}

// splitCommaSeparatedKV splits a non-JSON --json value on commas (values with commas must use JSON object form).
func splitCommaSeparatedKV(raw string) []string {
	if !strings.Contains(raw, ",") {
		return []string{raw}
	}
	return strings.Split(raw, ",")
}

func mergeKeyValue(cmd *cobra.Command, dst map[string]string, allowed map[string]struct{}, raw string) error {
	idx := strings.Index(raw, "=")
	if idx <= 0 {
		return fmt.Errorf("invalid key=value for --json (expected key=value): %q", raw)
	}
	key := strings.TrimSpace(raw[:idx])
	nk := normalizeFlagKey(cmd, key)
	if _, ok := allowed[nk]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownJSONFlagKey, key)
	}
	val := strings.TrimSpace(raw[idx+1:])
	dst[nk] = val
	return nil
}
