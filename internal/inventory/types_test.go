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

func TestStackSetPreservesInsertionOrder(t *testing.T) {
	set := NewStackSet()
	if !set.Add(Stack{InventoryID: 1, ItemID: 4, Amount: 1}, 0) {
		t.Fatalf("add first stack")
	}
	if !set.Add(Stack{InventoryID: 1, ItemID: 2, Amount: 1}, 0) {
		t.Fatalf("add second stack")
	}
	if !set.Add(Stack{InventoryID: 1, ItemID: 4, Amount: 2}, 0) {
		t.Fatalf("add existing stack")
	}

	items := set.Items(0)
	if len(items) != 2 || items[0].ItemID != 4 || items[0].Amount != 3 || items[1].ItemID != 2 {
		t.Fatalf("items: got %+v", items)
	}
}

func TestStackSetSlotLimitAllowsExistingStack(t *testing.T) {
	set := NewStackSet()
	if !set.Add(Stack{InventoryID: 1, ItemID: 1, Amount: 1}, 1) {
		t.Fatalf("add first stack")
	}
	if set.Add(Stack{InventoryID: 1, ItemID: 2, Amount: 1}, 1) {
		t.Fatalf("new stack should not fit")
	}
	if !set.Add(Stack{InventoryID: 1, ItemID: 1, Amount: 3}, 1) {
		t.Fatalf("existing stack should fit")
	}

	stack, ok := set.Get(1)
	if !ok || stack.Amount != 4 {
		t.Fatalf("stack: got %+v ok=%v", stack, ok)
	}
}

func TestStackSetRestoreRemovesNewStack(t *testing.T) {
	set := NewStackSet()
	if !set.Add(Stack{InventoryID: 1, ItemID: 1, Amount: 1}, 0) {
		t.Fatalf("add stack")
	}

	set.Restore(1, Stack{}, false)

	if set.Len() != 0 {
		t.Fatalf("len: got %d, want 0", set.Len())
	}
	if items := set.Items(0); len(items) != 0 {
		t.Fatalf("items: got %+v", items)
	}
}
