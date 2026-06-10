package main

import (
	"encoding/json"
	"testing"
)

func TestInspectParamsJSON(t *testing.T) {
	var params InspectParams
	if err := json.Unmarshal([]byte(`{"refresh":true}`), &params); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !params.Refresh {
		t.Fatalf("Refresh = false, want true")
	}

	if err := json.Unmarshal([]byte(`{"database":"appdb"}`), &params); err != nil {
		t.Fatalf("unmarshal database: %v", err)
	}
	if params.Database != "appdb" {
		t.Fatalf("Database = %q, want appdb", params.Database)
	}
}
