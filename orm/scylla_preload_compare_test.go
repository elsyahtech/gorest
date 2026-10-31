package orm

import "testing"

func TestCompareScyllaPreloadSortValues(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "int less", got: compareInt(1, 2), want: -1},
		{name: "int equal", got: compareInt(2, 2), want: 0},
		{name: "int greater", got: compareInt(3, 2), want: 1},
		{name: "uint less", got: compareUint(1, 2), want: -1},
		{name: "uint equal", got: compareUint(2, 2), want: 0},
		{name: "uint greater", got: compareUint(3, 2), want: 1},
		{name: "float less", got: compareFloat(1, 2), want: -1},
		{name: "float equal", got: compareFloat(2, 2), want: 0},
		{name: "float greater", got: compareFloat(3, 2), want: 1},
		{name: "bool less", got: compareBool(false, true), want: -1},
		{name: "bool equal", got: compareBool(true, true), want: 0},
		{name: "bool greater", got: compareBool(true, false), want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("comparison = %d, want %d", test.got, test.want)
			}
		})
	}
}
