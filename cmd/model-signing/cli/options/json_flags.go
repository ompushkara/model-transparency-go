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
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

// ErrUnknownJSONFlagKey is returned when --json contains a key that is not a defined
// (non-hidden) flag for the command being run.
var ErrUnknownJSONFlagKey = errors.New("unknown JSON key: not a defined flag for this command")

// JSONFlags defines a --json flag that accepts JSON objects and/or key=value pairs.
// Parse validates keys against flags already registered on the command (local and inherited).
type JSONFlags struct {
	// JSONData holds merged key/value pairs after a successful Parse.
	JSONData map[string]string

	jsonInputs []string
}

var _ FlagAdder = (*JSONFlags)(nil)

// NewJSONFlags returns an empty JSONFlags. Register other flags on the command first,
// then AddFlags, then call Parse(cmd) so allowed keys match existing flags.
func NewJSONFlags() *JSONFlags {
	return &JSONFlags{}
}

// AddFlags registers --json on the command's local flag set.
func (o *JSONFlags) AddFlags(cmd *cobra.Command) {
	o.registerJSON(cmd.Flags())
}

// AddPersistentFlags registers persistent --json (e.g. on the CLI root) so it is available on all subcommands.
func (o *JSONFlags) AddPersistentFlags(cmd *cobra.Command) {
	o.registerJSON(cmd.PersistentFlags())
}

func (o *JSONFlags) registerJSON(fs *flag.FlagSet) {
	fs.StringArrayVar(&o.jsonInputs, "json", nil,
		`Options as JSON object and/or key=value (repeat flag to merge). Keys must name flags valid for the command (hyphens or underscores, matching other flags). Explicit flags override --json.`)
}

// HasJSONInput reports whether any non-empty --json value was provided.
func (o *JSONFlags) HasJSONInput() bool {
	for _, s := range o.jsonInputs {
		if strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// ApplyParsed sets each parsed value on cmd's flags when that flag was not already set on the command line.
func (o *JSONFlags) ApplyParsed(cmd *cobra.Command) error {
	for k, v := range o.JSONData {
		f := cmd.Flag(k)
		if f == nil {
			return fmt.Errorf("internal error: flag %q not found after --json parse", k)
		}
		if !f.Changed {
			if err := f.Value.Set(v); err != nil {
				return fmt.Errorf("apply --json to flag %q: %w", k, err)
			}
		}
	}
	return nil
}

// ParseAndApply runs Parse then ApplyParsed. No-op when HasJSONInput is false.
func (o *JSONFlags) ParseAndApply(cmd *cobra.Command) error {
	if !o.HasJSONInput() {
		return nil
	}
	if err := o.Parse(cmd); err != nil {
		return err
	}
	return o.ApplyParsed(cmd)
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

// Parse merges and validates all --json values into JSONData.
// For JSON objects, every key must be a defined (non-hidden) flag on cmd; unknown keys
// fail with an error wrapping ErrUnknownJSONFlagKey.
func (o *JSONFlags) Parse(cmd *cobra.Command) error {
	allowed := allowedFlagNames(cmd)
	out := make(map[string]string)
	for _, raw := range o.jsonInputs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "{") {
			if err := mergeJSONObject(cmd, out, allowed, raw); err != nil {
				return err
			}
			continue
		}
		for _, part := range splitCommaSeparatedKV(raw) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if err := mergeKeyValue(cmd, out, allowed, part); err != nil {
				return err
			}
		}
	}
	o.JSONData = out
	return nil
}

func mergeJSONObject(cmd *cobra.Command, dst map[string]string, allowed map[string]struct{}, raw string) error {
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON for --json: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("invalid JSON for --json: must be a JSON object (got %v)", tok)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return fmt.Errorf("invalid JSON for --json: %w", err)
	}
	if err := validateJSONKeysAgainstAllowed(cmd, obj, allowed); err != nil {
		return err
	}
	for key, rawMsg := range obj {
		nk := normalizeFlagKey(cmd, key)
		s, err := stringifyJSONValue(rawMsg)
		if err != nil {
			return fmt.Errorf("key %q: %w", key, err)
		}
		dst[nk] = s
	}
	return nil
}

func validateJSONKeysAgainstAllowed(cmd *cobra.Command, obj map[string]json.RawMessage, allowed map[string]struct{}) error {
	for key := range obj {
		nk := normalizeFlagKey(cmd, key)
		if _, ok := allowed[nk]; !ok {
			return fmt.Errorf("%w: %q", ErrUnknownJSONFlagKey, key)
		}
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

// splitCommaSeparatedKV splits a non-JSON --json value on commas so that
// name=John,age=30,city=NYC works in one flag. Values containing commas must use JSON.
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
