package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var tagdowngrade = &cobra.Command{
	Use:   "downgrade <image tag>",
	Short: "Move a tag to an older generation",
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag")
		}

		svc, err := CreateTagService()
		if err != nil {
			return err
		}

		ns, err := namespace(c)
		if err != nil {
			return err
		}

		it, err := svc.Downgrade(context.Background(), ns, args[0])
		if err != nil {
			return err
		}

		log.Printf("tag %s downgraded (gen %d)", args[0], it.Spec.Generation)
		return nil
	},
}
