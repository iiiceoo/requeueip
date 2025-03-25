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

package install

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install RequeueIP CNI binary.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return copyFile(srcPath, binPath)
	},
}

func InstallCmd() *cobra.Command {
	return installCmd
}

const (
	srcPath = "/usr/bin/requeueip"
	binPath = "/host/opt/cni/bin/requeueip"
)

// copyFile copies file src to file dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Always remove the old CNI binary.
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	if err := dstFile.Sync(); err != nil {
		return err
	}

	return os.Chmod(dst, 0755)
}
