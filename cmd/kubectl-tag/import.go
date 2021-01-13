package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var tagimport = &cobra.Command{
	Use:   "import <image tag>",
	Short: "Imports a new generation for a tag",
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("provide an image tag")
		}

		svc, err := CreateTagService()
		if err != nil {
			return err
		}

		ns, err := Namespace(c)
		if err != nil {
			return err
		}

		it, err := svc.NewGeneration(context.Background(), ns, args[0])
		if err != nil {
			return err
		}

		log.Printf("tag %s imported (gen %d)", args[0], it.Spec.Generation)
		return nil
	},
}
