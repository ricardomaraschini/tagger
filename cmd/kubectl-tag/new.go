package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ricardomaraschini/tagger/cmd/kubectl-tag/static"
)

func init() {
	tagnew.Flags().StringP("namespace", "n", "", "Namespace to use")
	tagnew.Flags().StringP("from", "f", "", "From where to import the tag")
	tagnew.Flags().Bool("mirror", false, "Mirror the image into internal registry (mirror)")
	tagnew.MarkFlagRequired("from")
}

var tagnew = &cobra.Command{
	Use:     "new --from reg.io/repo/name:tag --mirror -n namespace tagname",
	Short:   "Imports a new generation for a tag",
	Long:    static.Text["new_help_header"],
	Example: static.Text["new_help_examples"],
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag name")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		svc, err := CreateTagService()
		if err != nil {
			return err
		}

		ns, err := Namespace(c)
		if err != nil {
			return err
		}

		mirror, err := c.Flags().GetBool("mirror")
		if err != nil {
			return err
		}

		from, err := c.Flags().GetString("from")
		if err != nil {
			return err
		}

		return svc.NewTag(ctx, ns, args[0], from, mirror)
	},
}
