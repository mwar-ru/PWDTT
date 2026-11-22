package backend

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.6.0-st.3", "1.6.0-st.2", true},
		{"1.6.0-st.2", "1.6.0-st.2", false},
		{"1.6.0-st.1", "1.6.0-st.2", false},
		{"1.6.0-st.2", "1.6.0", true},
		{"1.7.0-st.1", "1.6.0-st.9", true},
		{"1.5.9-st.9", "1.6.0-st.1", false},
	}
	for _, tc := range cases {
		if got := isNewer(tc.a, tc.b); got != tc.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
