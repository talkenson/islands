package inventory

import "testing"

func TestItemDefFitsPocket(t *testing.T) {
	axe := ItemDef{ID: 1, Tags: TagTool}
	if !axe.FitsPocket() {
		t.Fatalf("tool should fit pocket")
	}

	wood := ItemDef{ID: 2, Tags: TagBulkResource}
	if wood.FitsPocket() {
		t.Fatalf("bulk resource should not fit pocket")
	}
}
