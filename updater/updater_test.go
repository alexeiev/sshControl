package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	u := New("dev")

	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "dev is always older", current: "dev", latest: "v1.0.0", want: true},
		{name: "higher patch version", current: "v1.2.3", latest: "v1.2.4", want: true},
		{name: "higher minor version", current: "v1.2.9", latest: "v1.10.0", want: true},
		{name: "same version", current: "v1.2.3", latest: "v1.2.3", want: false},
		{name: "older latest version", current: "v2.0.0", latest: "v1.9.9", want: false},
		{name: "fallback lexical when non numeric", current: "v1.2.3-beta", latest: "v1.2.3", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := u.compareVersions(tt.current, tt.latest); got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestParseVersionParts(t *testing.T) {
	t.Parallel()

	got, ok := parseVersionParts("1.10.3")
	if !ok {
		t.Fatal("parseVersionParts returned ok=false for valid semver")
	}

	want := []int{1, 10, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseVersionParts = %v, want %v", got, want)
		}
	}

	if _, ok := parseVersionParts("1.2.beta"); ok {
		t.Fatal("parseVersionParts should reject non-numeric parts")
	}
}
