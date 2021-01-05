package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ricardomaraschini/tagger/services"
	"github.com/spf13/cobra"
)

var tagupgrade = &cobra.Command{
	Use:   "upgrade <image tag>",
	Short: "Move a tag to a newer generation",
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag")
		}

		cli, err := imagesCli()
		if err != nil {
			return err
		}

		ns, err := namespace(c)
		if err != nil {
			return err
		}

		svc := services.NewTag(nil, nil, cli, nil)
		it, err := svc.Upgrade(context.Background(), ns, args[0])
		if err != nil {
			return err
		}

		log.Printf("tag %s upgraded (gen %d)", args[0], it.Spec.Generation)
		return nil
	},
}
