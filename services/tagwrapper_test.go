package services

import (
	"strings"
	"testing"

	imagtagv1 "github.com/ricardomaraschini/tagger/imagetags/v1"
)

func TestSpecTagImported(t *testing.T) {
	for _, tt := range []struct {
		name string
		exp  bool
		tag  *imagtagv1.Tag
	}{
		{
			name: "empty tag",
			exp:  false,
			tag:  &imagtagv1.Tag{},
		},
		{
			name: "not imported yet",
			exp:  false,
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 2,
				},
				Status: imagtagv1.TagStatus{
					References: []imagtagv1.HashReference{
						{Generation: 1},
						{Generation: 0},
					},
				},
			},
		},
		{
			name: "tag already imported",
			exp:  true,
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 1,
				},
				Status: imagtagv1.TagStatus{
					References: []imagtagv1.HashReference{
						{Generation: 4},
						{Generation: 3},
						{Generation: 2},
						{Generation: 1},
						{Generation: 0},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tw := TagWrapper{tt.tag}
			res := tw.SpecTagImported()
			if res != tt.exp {
				t.Errorf("expected %v, %v received", tt.exp, res)
			}
		})
	}
}

func TestValidateTagGeneration(t *testing.T) {
	for _, tt := range []struct {
		name string
		tag  *imagtagv1.Tag
		err  string
	}{
		{
			name: "empty tag",
			tag:  &imagtagv1.Tag{},
		},
		{
			name: "invalid generation",
			err:  "generation must be one of: [0]",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 2,
				},
			},
		},
		{
			name: "next generation",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 10,
				},
				Status: imagtagv1.TagStatus{
					References: []imagtagv1.HashReference{
						{Generation: 9},
						{Generation: 8},
					},
				},
			},
		},
		{
			name: "one old but valid generation",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: 2,
				},
				Status: imagtagv1.TagStatus{
					References: []imagtagv1.HashReference{
						{Generation: 5},
						{Generation: 4},
						{Generation: 3},
						{Generation: 2},
					},
				},
			},
		},
		{
			name: "negative generation",
			err:  "generation must be one of: [0]",
			tag: &imagtagv1.Tag{
				Spec: imagtagv1.TagSpec{
					Generation: -1,
				},
				Status: imagtagv1.TagStatus{
					References: nil,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tw := TagWrapper{tt.tag}
			err := tw.ValidateTagGeneration()
			if err != nil {
				if len(tt.err) == 0 {
					t.Errorf("unexpected error %s", err)
					return
				}
				if !strings.Contains(err.Error(), tt.err) {
					t.Errorf("invalid error %s", err)
				}
				return
			} else if len(tt.err) > 0 {
				t.Errorf("expecting %q, nil received instead", tt.err)
			}
		})
	}
}
