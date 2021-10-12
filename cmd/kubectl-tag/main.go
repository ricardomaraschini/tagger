// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/containers/storage/pkg/reexec"
	"github.com/containers/storage/pkg/unshare"
	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/infra/fs"
	itagcli "github.com/ricardomaraschini/tagger/infra/tags/v1beta1/gen/clientset/versioned"
	"github.com/ricardomaraschini/tagger/services"
)

func main() {
	if reexec.Init() {
		panic("reexec returned true")
	}
	unshare.MaybeReexecUsingUserNamespace(true)

	sigs := []os.Signal{syscall.SIGTERM, syscall.SIGINT}
	ctx, cancel := signal.NotifyContext(context.Background(), sigs...)
	defer cancel()

	root := &cobra.Command{
		Use:          "kubectl-tag",
		SilenceUsage: true,
	}
	root.AddCommand(
		tagupgrade, tagimport, tagversion, tagpush, tagdowngrade,
		tagpull, tagmirror, tagnew, taglocalpush,
	)
	root.ExecuteContext(ctx)
}

// createTagService creates and returns a services.Tag struct.
func createTagService() (*services.Tag, error) {
	cfgpath := os.Getenv("KUBECONFIG")

	config, err := clientcmd.BuildConfigFromFlags("", cfgpath)
	if err != nil {
		return nil, fmt.Errorf("error building config: %s", err)
	}

	tagcli, err := itagcli.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return services.NewTag(nil, tagcli, nil), nil
}

// namespace returns the namespace provided through the --namespace/-n command
// line flag or the default one as extracted from kube configuration.
func namespace(c *cobra.Command) (string, error) {
	nsflag := c.Flag("namespace")
	if nsflag != nil && nsflag.Value.String() != "" {
		return nsflag.Value.String(), nil
	}

	cfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return "", err
	}
	return cfg.Contexts[cfg.CurrentContext].Namespace, nil
}

// createHomeTempDir creates a directory in user's home directory. Creates and return a
// fs.FS handler using the created directory.
func createHomeTempDir() (*fs.FS, error) {
	hdir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	tmpdir := path.Join(hdir, ".tagger")
	if _, err := os.Stat(tmpdir); os.IsNotExist(err) {
		if err := os.Mkdir(tmpdir, 0700); err != nil {
			return nil, err
		}
	}

	return fs.New(tmpdir), nil
}
