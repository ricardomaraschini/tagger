package main

import (
	"fmt"
	"log"
	"os"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/containers/storage/pkg/reexec"
	"github.com/containers/storage/pkg/unshare"
	"github.com/spf13/cobra"

	itagcli "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned"
	"github.com/ricardomaraschini/tagger/services"
)

func main() {
	if reexec.Init() {
		panic("reexec returned true")
	}
	unshare.MaybeReexecUsingUserNamespace(true)

	root := &cobra.Command{Use: "kubectl-tag"}
	root.AddCommand(tagupgrade)
	root.AddCommand(tagdowngrade)
	root.AddCommand(tagimport)
	root.AddCommand(tagpull)
	root.AddCommand(tagpush)
	root.AddCommand(tagnew)
	if err := root.Execute(); err != nil {
		log.Print(err)
	}
}

// CreateTagService creates and returns a services.Tag struct.
func CreateTagService() (*services.Tag, error) {
	cfgpath := os.Getenv("KUBECONFIG")

	config, err := clientcmd.BuildConfigFromFlags("", cfgpath)
	if err != nil {
		return nil, fmt.Errorf("error building config: %s", err)
	}

	tagcli, err := itagcli.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return services.NewTag(nil, nil, tagcli, nil), nil
}

// Namespace returns the namespace provided through the --namespace/-n command
// line flag or the default one as extracted from kube configuration.
func Namespace(c *cobra.Command) (string, error) {
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
