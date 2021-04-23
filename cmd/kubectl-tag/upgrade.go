package main

import (
	"log"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagupgrade.Flags().StringP("namespace", "n", "", "namespace to use")
}

var tagupgrade = &cobra.Command{
	Use:     "upgrade <image tag>",
	Short:   "Moves a tag to a newer generation",
	Long:    static.Text["upgrade_help_header"],
	Example: static.Text["upgrade_help_examples"],
	Run: func(c *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Fatal("provide an image tag")
		}

		svc, err := createTagService()
		if err != nil {
			log.Fatal(err)
		}

		ns, err := namespace(c)
		if err != nil {
			log.Fatal(err)
		}

		it, err := svc.Upgrade(c.Context(), ns, args[0])
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("tag %s upgraded (gen %d)", args[0], it.Spec.Generation)
	},
}
