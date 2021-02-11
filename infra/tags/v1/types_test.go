package v1

import (
	"reflect"
	"strings"
	"testing"
)

func TestPrependHashReference(t *testing.T) {
	for _, tt := range []struct {
		name      string
		current   []HashReference
		reference HashReference
		expected  []HashReference
	}{
		{
			name:    "nil current generations slice",
			current: nil,
			reference: HashReference{
				Generation: 1,
			},
			expected: []HashReference{
				{Generation: 1},
			},
		},
		{
			name:    "empty current generations slice",
			current: []HashReference{},
			reference: HashReference{
				Generation: 1,
			},
			expected: []HashReference{
				{Generation: 1},
			},
		},
		{
			name: "full current generations slice",
			current: []HashReference{
				{Generation: 4},
				{Generation: 3},
				{Generation: 2},
				{Generation: 1},
				{Generation: 0},
			},
			reference: HashReference{
				Generation: 5,
			},
			expected: []HashReference{
				{Generation: 5},
				{Generation: 4},
				{Generation: 3},
				{Generation: 2},
				{Generation: 1},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tag := &Tag{
				Status: TagStatus{
					References: tt.current,
				},
			}
			tag.PrependHashReference(tt.reference)
			if reflect.DeepEqual(tag.Status.References, tt.expected) {
				return
			}
			t.Errorf("expected %+v, %+v received", tt.expected, tag.Status.References)
		})
	}
}

func TestSpecTagImported(t *testing.T) {
	for _, tt := range []struct {
		name string
		exp  bool
		tag  *Tag
	}{
		{
			name: "empty tag",
			exp:  false,
			tag:  &Tag{},
		},
		{
			name: "not imported yet",
			exp:  false,
			tag: &Tag{
				Spec: TagSpec{
					Generation: 2,
				},
				Status: TagStatus{
					References: []HashReference{
						{Generation: 1},
						{Generation: 0},
					},
				},
			},
		},
		{
			name: "tag already imported",
			exp:  true,
			tag: &Tag{
				Spec: TagSpec{
					Generation: 1,
				},
				Status: TagStatus{
					References: []HashReference{
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
			if res := tt.tag.SpecTagImported(); res != tt.exp {
				t.Errorf("expected %v, %v received", tt.exp, res)
			}
		})
	}
}

func TestValidateTagGeneration(t *testing.T) {
	for _, tt := range []struct {
		name string
		tag  *Tag
		err  string
	}{
		{
			name: "empty tag",
			tag:  &Tag{},
		},
		{
			name: "invalid generation",
			err:  "generation must be one of: [0]",
			tag: &Tag{
				Spec: TagSpec{
					Generation: 2,
				},
			},
		},
		{
			name: "next generation",
			tag: &Tag{
				Spec: TagSpec{
					Generation: 10,
				},
				Status: TagStatus{
					References: []HashReference{
						{Generation: 9},
						{Generation: 8},
					},
				},
			},
		},
		{
			name: "one old but valid generation",
			tag: &Tag{
				Spec: TagSpec{
					Generation: 2,
				},
				Status: TagStatus{
					References: []HashReference{
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
			tag: &Tag{
				Spec: TagSpec{
					Generation: -1,
				},
				Status: TagStatus{
					References: nil,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.tag.ValidateTagGeneration(); err != nil {
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
