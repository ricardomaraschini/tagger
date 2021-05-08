package main

import (
	"log"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagmirror.Flags().StringP("namespace", "n", "", "namespace to use")
}

var tagmirror = &cobra.Command{
	Use:     "mirror registry.io/repo/name:tag -n namespace tagname",
	Short:   "Mirrors a remote image into a tag",
	Long:    static.Text["mirror_help_header"],
	Example: static.Text["mirror_help_examples"],
	Run: func(c *cobra.Command, args []string) {
		if len(args) != 2 {
			log.Fatal("provide an image source and a tag name")
		}

		svc, err := createTagService()
		if err != nil {
			log.Fatal(err)
			return
		}

		ns, err := namespace(c)
		if err != nil {
			log.Fatal(err)
		}

		if err := svc.NewTag(
			c.Context(), ns, args[1], args[0], true,
		); err != nil {
			log.Fatal(err)
		}
	},
}
