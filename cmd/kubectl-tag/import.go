package main

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"
)

var tagimport = &cobra.Command{
	Use:   "import <image tag>",
	Short: "Imports a new generation for a tag",
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

		tag, err := cli.ImagesV1().Tags(ns).Get(
			context.Background(), args[0], metav1.GetOptions{},
		)
		if err != nil {
			return err
		}

		nextGen := int64(0)
		if len(tag.Status.References) > 0 {
			nextGen = tag.Status.References[0].Generation + 1
		}
		tag.Spec.Generation = nextGen

		if tag, err = cli.ImagesV1().Tags(ns).Update(
			context.Background(), tag, metav1.UpdateOptions{},
		); err != nil {
			return err
		}

		log.Printf("tag %s imported (gen %d)", args[0], tag.Spec.Generation)
		return nil
	},
}
