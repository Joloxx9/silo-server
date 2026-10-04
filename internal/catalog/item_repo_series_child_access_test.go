package catalog

import (
	"strings"
	"testing"
)

// A season and an episode are not rows in media_items, so the probe used by
// the ratings path has to reach the parent series that carries library
// membership. Resolving only one of the two is the defect this guards: episode
// ratings worked while season ratings answered "not found".
func TestAccessibleSQLResolvesBothSeriesChildren(t *testing.T) {
	query, args := buildAccessibleSQL("season:ozark-1", AccessFilter{}, true)

	if !strings.Contains(query, "FROM episodes e_parent") {
		t.Errorf("probe does not resolve an episode to its series:\n%s", query)
	}
	if !strings.Contains(query, "FROM seasons s_parent") {
		t.Errorf("probe does not resolve a season to its series:\n%s", query)
	}
	// One placeholder serves the id in every branch, so a caller that bound
	// it once still binds exactly one argument.
	if len(args) != 1 || args[0] != "season:ozark-1" {
		t.Errorf("expected the content id bound once, got %#v", args)
	}
}

// Every other caller keeps the plain equality check: resolving parents for
// favorites or history would let a child id stand in for its series.
func TestAccessibleSQLWithoutResolutionMatchesOwnIDOnly(t *testing.T) {
	query, _ := buildAccessibleSQL("movie:heat-1995", AccessFilter{}, false)

	if strings.Contains(query, "e_parent") || strings.Contains(query, "s_parent") {
		t.Errorf("unresolved probe should not consult child tables:\n%s", query)
	}
}
