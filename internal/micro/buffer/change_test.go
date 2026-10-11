package buffer

import "testing"

func TestTextChangeVersionsCoverEditUndoRedo(t *testing.T) {
	left := NewBufferFromString("", "", BTTransient)
	right := NewBufferFromString("", "", BTTransient)
	defer left.Close()
	defer right.Close()

	var leftChanges, rightChanges []TextChange
	left.SetChangeHandler(func(change TextChange) { leftChanges = append(leftChanges, change) })
	right.SetChangeHandler(func(change TextChange) { rightChanges = append(rightChanges, change) })

	left.Insert(Loc{X: 0, Y: 0}, "alpha\nbeta")
	if len(leftChanges) != 1 || leftChanges[0] != (TextChange{Version: 1, InsertedBytes: 10, InsertedNewlines: 1}) {
		t.Fatalf("insert notification = %#v", leftChanges)
	}
	if right.ContentVersion() != 0 || len(rightChanges) != 0 {
		t.Fatalf("independent buffer changed: version=%d changes=%#v", right.ContentVersion(), rightChanges)
	}

	left.Remove(Loc{X: 0, Y: 0}, Loc{X: 5, Y: 0})
	if left.ContentVersion() != 2 || leftChanges[1].InsertedBytes != 0 {
		t.Fatalf("remove notification = %#v", leftChanges)
	}
	left.UndoOneEvent()
	if left.ContentVersion() != 3 || leftChanges[2].InsertedBytes != 5 {
		t.Fatalf("undo notification = %#v", leftChanges)
	}
	left.RedoOneEvent()
	if left.ContentVersion() != 4 || leftChanges[3].InsertedBytes != 0 {
		t.Fatalf("redo notification = %#v", leftChanges)
	}
	if right.ContentVersion() != 0 {
		t.Fatalf("right version advanced to %d", right.ContentVersion())
	}
}
