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
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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
