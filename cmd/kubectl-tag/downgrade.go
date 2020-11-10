package main

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"
)

var tagdowngrade = &cobra.Command{
	Use:   "downgrade <image tag>",
	Short: "Move a tag to an older generation",
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

		expectedGen := tag.Spec.Generation - 1
		found := false
		for _, ref := range tag.Status.References {
			if ref.Generation != expectedGen {
				continue
			}
			found = true
			break
		}

		if !found {
			log.Print("unable to downgrade, currently at oldest generation")
			return nil
		}

		tag.Spec.Generation = expectedGen
		if tag, err = cli.ImagesV1().Tags(ns).Update(
			context.Background(), tag, metav1.UpdateOptions{},
		); err != nil {
			return err
		}

		log.Printf("tag %s downgraded (gen %d)", args[0], tag.Spec.Generation)
		return nil
	},
}
