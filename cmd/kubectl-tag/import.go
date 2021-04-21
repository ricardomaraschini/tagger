package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagimport.Flags().StringP("namespace", "n", "", "namespace to use")
}

var tagimport = &cobra.Command{
	Use:     "import <image tag>",
	Short:   "Imports a new generation for a tag",
	Long:    static.Text["import_help_header"],
	Example: static.Text["import_help_examples"],
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

		it, err := svc.NewGeneration(context.Background(), ns, args[0])
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("tag %s imported (gen %d)", args[0], it.Spec.Generation)
	},
}
