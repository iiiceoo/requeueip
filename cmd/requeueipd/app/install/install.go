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
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install RequeueIP CNI binary.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return install(ctrl.SetupSignalHandler())
	},
}

func InstallCmd() *cobra.Command {
	return installCmd
}

const (
	srcPath = "/usr/bin/requeueip"
	binPath = "/host/opt/cni/bin/requeueip"
)

func install(ctx context.Context) error {
	return copyFile(ctx, srcPath, binPath)
}

// copyFile copies file src to file dst.
func copyFile(ctx context.Context, src, dst string) error {
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

	errCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(dstFile, srcFile)
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		if err := os.Remove(dst); err != nil {
			return err
		}
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			return err
		}
	}

	if err := dstFile.Sync(); err != nil {
		return err
	}

	return os.Chmod(dst, 0755)
}
