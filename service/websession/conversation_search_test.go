package websession

import (
	"context"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestConversationSearchPreservesTextForOccurrenceNavigation(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	project := seedProject(t)
	session := seedWebSession(t, project.ID, "Repeated search hits", 1000)
	rows := []tables.WebSessionItemTable{
		{WebSessionID: session.ID, OrderIndex: 1, ItemKind: "user", Text: "plan plan"},
		{WebSessionID: session.ID, OrderIndex: 2, ItemKind: "assistant", Text: "plan **PLAN** [plan](https://example.com/plan)"},
	}
	for index := range rows {
		rows[index].Init()
		if err := model.GetDB().Create(&rows[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.SearchSessionConversation(context.Background(), session.ID, "plan", true, true, false, false, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].Text != rows[1].Text || first.Done || first.NextCursor == "" {
		t.Fatalf("expected original Markdown and a history cursor, got %+v", first)
	}
	second, err := manager.SearchSessionConversation(context.Background(), session.ID, "plan", true, true, false, false, "", first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Text != rows[0].Text || !second.Done {
		t.Fatalf("expected repeated user text on the final page, got %+v", second)
	}
}
