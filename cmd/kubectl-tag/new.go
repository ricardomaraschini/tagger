package main

import (
	"context"
	"log"
	"time"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagnew.Flags().StringP("namespace", "n", "", "namespace to use")
	tagnew.Flags().StringP("from", "f", "", "from where to import the tag")
	tagnew.Flags().Bool("mirror", false, "mirror the image")
	tagnew.MarkFlagRequired("from")
}

var tagnew = &cobra.Command{
	Use:     "new --from reg.io/repo/name:tag --mirror -n namespace tagname",
	Short:   "Creates a new tag by importing it from a remote registry",
	Long:    static.Text["new_help_header"],
	Example: static.Text["new_help_examples"],
	Run: func(c *cobra.Command, args []string) {
		if len(args) != 1 {
			log.Fatal("provide an image tag name")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		svc, err := createTagService()
		if err != nil {
			log.Fatal(err)
			return
		}

		ns, err := namespace(c)
		if err != nil {
			log.Fatal(err)
		}

		mirror, err := c.Flags().GetBool("mirror")
		if err != nil {
			log.Fatal(err)
		}

		from, err := c.Flags().GetString("from")
		if err != nil {
			log.Fatal(err)
		}

		if err := svc.NewTag(ctx, ns, args[0], from, mirror); err != nil {
			log.Fatal(err)
		}
	},
}
