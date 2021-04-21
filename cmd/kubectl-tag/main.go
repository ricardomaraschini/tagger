package main

import (
	"fmt"
	"os"
	"os/exec"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/containers/storage/pkg/reexec"
	"github.com/containers/storage/pkg/unshare"
	"github.com/hashicorp/go-multierror"
	"github.com/spf13/cobra"

	itagcli "github.com/ricardomaraschini/tagger/infra/tags/v1/gen/clientset/versioned"
	"github.com/ricardomaraschini/tagger/services"
)

// List of container runtimes.
const (
	UnknownRuntime = iota
	PodmanRuntime
	DockerRuntime
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
	root.Execute()
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

// containerRuntime returns the container runtime installed in the operating
// system. We do an attempt to look for a binary called 'podman' or 'docker'
// in the host PATH environment variable.
func containerRuntime() (int, error) {
	var errors *multierror.Error

	_, err := exec.LookPath("podman")
	if err == nil {
		return PodmanRuntime, nil
	}
	errors = multierror.Append(errors, err)

	_, err = exec.LookPath("docker")
	if err == nil {
		return DockerRuntime, nil
	}
	errors = multierror.Append(errors, err)

	err = fmt.Errorf("unable to determine container runtime: %w", errors)
	return UnknownRuntime, err
}
