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
	"encoding/json"
	"strings"
	"testing"
)

func TestRootOptions_ValidateOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{name: "default empty", output: "", wantErr: false},
		{name: "text", output: "text", wantErr: false},
		{name: "json", output: "json", wantErr: false},
		{name: "JSON case", output: "JSON", wantErr: false},
		{name: "spaces", output: "  json  ", wantErr: false},
		{name: "invalid", output: "yaml", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &RootOptions{Output: tt.output}
			err := o.ValidateOutput()
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRootOptions_ResultOutputFormat(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{output: "", want: "text"},
		{output: "text", want: "text"},
		{output: "TEXT", want: "text"},
		{output: "json", want: "json"},
		{output: " JSON ", want: "json"},
	}
	for _, tt := range tests {
		o := &RootOptions{Output: tt.output}
		if got := o.ResultOutputFormat(); got != tt.want {
			t.Errorf("ResultOutputFormat(%q) = %q, want %q", tt.output, got, tt.want)
		}
	}
}

func TestRootOptions_SignVerifyResultLine(t *testing.T) {
	tests := []struct {
		name      string
		opts      RootOptions
		verified  bool
		message   string
		wantLine  string
		wantEmit  bool
		wantJSON  bool // if true, wantLine must unmarshal as signVerifyResultJSON
		checkJSON func(t *testing.T, raw string)
	}{
		{
			name:     "silent skips",
			opts:     RootOptions{LogLevel: "silent", Output: "json"},
			verified: true,
			message:  "ok",
			wantLine: "",
			wantEmit: false,
		},
		{
			name:     "text info",
			opts:     RootOptions{LogLevel: "info", Output: "text"},
			verified: true,
			message:  "Verification succeeded",
			wantLine: "Verification succeeded",
			wantEmit: true,
		},
		{
			name:     "json info",
			opts:     RootOptions{LogLevel: "info", Output: "json"},
			verified: true,
			message:  "Verification succeeded",
			wantEmit: true,
			wantJSON: true,
			checkJSON: func(t *testing.T, raw string) {
				t.Helper()
				var got signVerifyResultJSON
				if err := json.Unmarshal([]byte(raw), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !got.Verified || got.Message != "Verification succeeded" {
					t.Fatalf("got %+v", got)
				}
			},
		},
		{
			name:     "json failure fields",
			opts:     RootOptions{LogLevel: "warn", Output: "json"},
			verified: false,
			message:  "bad: x",
			wantEmit: true,
			wantJSON: true,
			checkJSON: func(t *testing.T, raw string) {
				t.Helper()
				var got signVerifyResultJSON
				if err := json.Unmarshal([]byte(raw), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if got.Verified || got.Message != "bad: x" {
					t.Fatalf("got %+v", got)
				}
			},
		},
		{
			name:     "json escapes message",
			opts:     RootOptions{LogLevel: "debug", Output: "json"},
			verified: false,
			message:  `quote"and\nline`,
			wantEmit: true,
			wantJSON: true,
			checkJSON: func(t *testing.T, raw string) {
				t.Helper()
				var got signVerifyResultJSON
				if err := json.Unmarshal([]byte(raw), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if got.Message != `quote"and\nline` {
					t.Fatalf("message got %q", got.Message)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, emit, err := tt.opts.SignVerifyResultLine(tt.verified, tt.message)
			if err != nil {
				t.Fatalf("SignVerifyResultLine: %v", err)
			}
			if emit != tt.wantEmit {
				t.Fatalf("emit = %v, want %v", emit, tt.wantEmit)
			}
			if !tt.wantJSON && line != tt.wantLine {
				t.Fatalf("line = %q, want %q", line, tt.wantLine)
			}
			if tt.wantJSON && tt.checkJSON != nil {
				tt.checkJSON(t, line)
			}
			if tt.wantJSON && !strings.HasPrefix(line, "{") {
				t.Fatalf("expected JSON object, got %q", line)
			}
		})
	}
}
