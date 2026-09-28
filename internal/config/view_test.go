package config

import "testing"

func TestValidateView(t *testing.T) {
	for _, ok := range []string{"", "list", "tree"} {
		if err := ValidateView(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if ValidateView("grid") == nil {
		t.Error("grid is not a view")
	}
}

func TestViewsAreDescribedAndIncludeTheDefault(t *testing.T) {
	if _, ok := Views[DefaultView]; !ok {
		t.Errorf("the default %q is not a view", DefaultView)
	}
	for _, name := range ViewNames() {
		if Views[name] == "" {
			t.Errorf("%q has no description", name)
		}
	}
}
