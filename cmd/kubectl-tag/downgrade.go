package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagdowngrade.Flags().StringP("namespace", "n", "", "namespace to use")
}

var tagdowngrade = &cobra.Command{
	Use:     "downgrade <image tag>",
	Short:   "Moves a tag to an older generation",
	Long:    static.Text["downgrade_help_header"],
	Example: static.Text["downgrade_help_examples"],
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

		it, err := svc.Downgrade(context.Background(), ns, args[0])
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("tag %s downgraded (gen %d)", args[0], it.Spec.Generation)
	},
}
