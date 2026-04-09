package buffer

import "testing"

func TestPositionAfter(t *testing.T) {
	cases := []struct {
		a, b Position
		want bool
	}{
		{pos(0, 1), pos(0, 0), true},
		{pos(0, 0), pos(0, 1), false},
		{pos(1, 0), pos(0, 5), true},
		{pos(0, 5), pos(1, 0), false},
		{pos(2, 3), pos(2, 3), false}, // equal
	}
	for _, c := range cases {
		if got := c.a.After(c.b); got != c.want {
			t.Errorf("%+v.After(%+v)=%v want %v",
				c.a, c.b, got, c.want)
		}
	}
}

func TestPositionAfterBeforeSymmetry(t *testing.T) {
	pairs := [][2]Position{
		{pos(0, 0), pos(0, 1)},
		{pos(0, 5), pos(1, 0)},
		{pos(3, 3), pos(3, 3)},
		{pos(10, 0), pos(0, 99)},
	}
	for _, p := range pairs {
		a, b := p[0], p[1]
		if a.Before(b) != b.After(a) {
			t.Errorf("symmetry broken: %+v.Before(%+v)=%v, "+
				"%+v.After(%+v)=%v",
				a, b, a.Before(b), b, a, b.After(a))
		}
		if a.After(b) != b.Before(a) {
			t.Errorf("symmetry broken: %+v.After(%+v)=%v, "+
				"%+v.Before(%+v)=%v",
				a, b, a.After(b), b, a, b.Before(a))
		}
	}
}
