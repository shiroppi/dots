package apply

import (
	"reflect"
	"testing"

	"github.com/shiroppi/dots/internal/model"
)

func TestCurrentTargetsIncludesOverriddenRules(t *testing.T) {
	res := &model.Resolution{
		Entries: []model.Entry{{Target: "/h/.a"}, {Target: "/h/.b"}},
		Overrides: []model.Override{
			{Target: "/h/.b", Loser: model.Entry{Target: "/h/.b"}},
			{Target: "/h/.c", Loser: model.Entry{Target: "/h/.c"}},
		},
	}
	want := []string{"/h/.a", "/h/.b", "/h/.b", "/h/.c"}
	if got := CurrentTargets(res); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}
