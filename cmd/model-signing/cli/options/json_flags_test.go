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
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestJSONFlags_ResolveModelPath(t *testing.T) {
	j := NewJSONFlags()
	j.modelPathFromJSON = "/from-json"
	got, err := j.ResolveModelPath(nil)
	if err != nil || got != "/from-json" {
		t.Fatalf("ResolveModelPath(nil) = %q, %v", got, err)
	}
	got, err = j.ResolveModelPath([]string{"/pos"})
	if err != nil || got != "/pos" {
		t.Fatalf("ResolveModelPath(pos) = %q, %v", got, err)
	}
	j.modelPathFromJSON = ""
	_, err = j.ResolveModelPath(nil)
	if err == nil {
		t.Fatal("expected error when no path")
	}
}

func TestJSONFlags_ParseAndApply_satisfiesRequired(t *testing.T) {
	root := &cobra.Command{Use: "root", TraverseChildren: true, SilenceUsage: true}
	jso := NewJSONFlags()
	jso.AddPersistentFlags(root)
	sub := &cobra.Command{
		Use:          "sub",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	sub.Flags().String("signature", "", "")
	_ = sub.MarkFlagRequired("signature")
	root.AddCommand(sub)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		return jso.ParseAndApply(cmd)
	}
	root.SetArgs([]string{"sub", "--json", `{"signature":"/tmp/x"}`})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializeJSONArg_stdin(t *testing.T) {
	var consumed bool
	got, err := materializeJSONArg("-", strings.NewReader(`  {"a": 1}  `), &consumed)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"a": 1}` {
		t.Fatalf("got %q", got)
	}
	if !consumed {
		t.Fatal("expected stdin consumed")
	}
}

func TestMaterializeJSONArg_secondStdinFails(t *testing.T) {
	var consumed bool
	_, err := materializeJSONArg("-", strings.NewReader(`{}`), &consumed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = materializeJSONArg("-", strings.NewReader(`{}`), &consumed)
	if err == nil {
		t.Fatal("expected error on second --json -")
	}
}

func TestMaterializeJSONArg_emptyStdin(t *testing.T) {
	var consumed bool
	_, err := materializeJSONArg("-", strings.NewReader("  \n  "), &consumed)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJSONFlags_parseWithStdin_JSONObject(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("signature", "", "sig path")

	j := &JSONFlags{jsonInputs: []string{"-"}}
	stdin := strings.NewReader(`{"signature":"/tmp/model.sig"}`)
	data, err := j.parseWithStdin(cmd, stdin)
	if err != nil {
		t.Fatal(err)
	}
	if data["signature"] != "/tmp/model.sig" {
		t.Fatalf("got %#v", data)
	}
}

func TestJSONFlags_parseWithStdin_keyValue(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("log_level", "", "")

	j := &JSONFlags{jsonInputs: []string{"-"}}
	stdin := strings.NewReader(`log_level=debug`)
	data, err := j.parseWithStdin(cmd, stdin)
	if err != nil {
		t.Fatal(err)
	}
	if data["log_level"] != "debug" {
		t.Fatalf("got %#v", data)
	}
}

func TestJSONFlags_parseWithStdin_inlineUnchanged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("signature", "", "")

	j := &JSONFlags{jsonInputs: []string{`{"signature":"/inline"}`}}
	data, err := j.parseWithStdin(cmd, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if data["signature"] != "/inline" {
		t.Fatalf("got %#v", data)
	}
}

func TestJSONFlags_parseWithStdin_modelKeyReserved(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("signature", "", "")

	j := &JSONFlags{jsonInputs: []string{`{"model":"/m","signature":"/s"}`}}
	data, err := j.parseWithStdin(cmd, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	got, rerr := j.ResolveModelPath(nil)
	if rerr != nil || got != "/m" {
		t.Fatalf("ResolveModelPath(nil) = %q, %v", got, rerr)
	}
	if _, ok := data[JSONModelPathKey]; ok {
		t.Fatalf("model should not remain in data map: %#v", data)
	}
	if data["signature"] != "/s" {
		t.Fatalf("got %#v", data)
	}
}

func TestJSONFlags_parseWithStdin_modelKeyValueForm(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("signature", "", "")

	j := &JSONFlags{jsonInputs: []string{`model=/fromkv,signature=/sig`}}
	data, err := j.parseWithStdin(cmd, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	got, rerr := j.ResolveModelPath(nil)
	if rerr != nil || got != "/fromkv" {
		t.Fatalf("ResolveModelPath(nil) = %q, %v", got, rerr)
	}
	if data["signature"] != "/sig" {
		t.Fatalf("got %#v", data)
	}
}

func TestJSONFlags_parseWithStdin_unknownKeyStillFails(t *testing.T) {
	cmd := &cobra.Command{}
	j := &JSONFlags{jsonInputs: []string{`{"model":"/m","not-a-flag":1}`}}
	_, err := j.parseWithStdin(cmd, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected unknown key error")
	}
	if !errors.Is(err, ErrUnknownJSONFlagKey) {
		t.Fatalf("got %v", err)
	}
}
