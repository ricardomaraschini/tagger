package main

import (
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/spf13/cobra"

	itagcli "github.com/ricardomaraschini/tagger/imagetags/generated/clientset/versioned"
)

func main() {
	root := &cobra.Command{Use: "kubectl-tag"}
	root.PersistentFlags().StringP(
		"namespace", "n", "", "Namespace to use",
	)

	root.AddCommand(tagupgrade)
	root.AddCommand(tagdowngrade)
	root.AddCommand(tagimport)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// imagesCli returns a client to access image tags through kubernetes api.
func imagesCli() (*itagcli.Clientset, error) {
	cfgpath := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", cfgpath)
	if err != nil {
		return nil, fmt.Errorf("error building config: %s", err)
	}
	return itagcli.NewForConfig(config)
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
