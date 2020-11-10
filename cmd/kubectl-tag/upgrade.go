package main

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

		tag, err := cli.ImagesV1().Tags(ns).Get(
			context.Background(), args[0], metav1.GetOptions{},
		)
		if err != nil {
			return err
		}

		tag.Spec.Generation++

		if tag, err = cli.ImagesV1().Tags(ns).Update(
			context.Background(), tag, metav1.UpdateOptions{},
		); err != nil {
			log.Fatalf("error updating tag: %s", err)
		}

		log.Printf("tag %s upgraded (gen %d)", args[0], tag.Spec.Generation)
		return nil
	},
}
