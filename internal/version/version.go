/*
Copyright 2024 The RequeueIP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

// Info contains versioning information.
type Info struct {
	Version   string `json:"Version"`
	GitCommit string `json:"Commit"`
	BuildDate string `json:"Build"`
	GoVersion string `json:"Go"`
	Platform  string `json:"Platform"`
}

// String returns info as a human-friendly version string.
func (info *Info) String() string {
	if s, err := info.Text(); err == nil {
		return s
	}

	return info.Version
}

// ToJSON returns the JSON string of version information.
func (info *Info) ToJSON() string {
	s, _ := json.Marshal(info)

	return string(s)
}

// Text encodes the version information into a human readable format.
func (info *Info) Text() (string, error) {
	text := strings.Builder{}
	text.WriteString("Version: " + info.Version + "\n")
	text.WriteString("Commit: " + info.GitCommit + "\n")
	text.WriteString("Build: " + info.BuildDate + "\n")
	text.WriteString("Go: " + info.GoVersion + "\n")
	text.WriteString("Platform: " + info.Platform)

	return text.String(), nil
}

// Get returns the overall codebase version. It's for detecting what code a
// binary was built from.
func Get() *Info {
	// These variables typically come from -ldflags settings and in their
	// absence fallback to the settings in internal/version/base.go.
	return &Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
