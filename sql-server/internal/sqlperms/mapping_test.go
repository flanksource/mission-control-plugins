package sqlperms

import (
	"strings"
	"testing"
)

func TestBuildReportSysadminGrantsEverything(t *testing.T) {
	r := BuildReport(probe{login: "sa", isSysadmin: true, engineEdition: 3}, "msdb")
	if !r.AllGranted() {
		t.Fatalf("sysadmin should have all categories granted, got %+v", r.Categories)
	}
	for _, c := range r.Categories {
		if len(c.GrantStatements) != 0 || len(c.MissingPermissions) != 0 {
			t.Errorf("%s: sysadmin should need no grants, got missing=%v grants=%v", c.Category, c.MissingPermissions, c.GrantStatements)
		}
		if !strings.Contains(c.Note, "sysadmin") {
			t.Errorf("%s: expected sysadmin note, got %q", c.Category, c.Note)
		}
	}
}

func TestBuildReportCategoriesMatchSqlPermissionReport(t *testing.T) {
	r := BuildReport(probe{login: "app_login", currentDatabase: "OIPA", engineEdition: 2}, "msdb")
	want := []Category{CategoryMetrics, CategoryInspection, CategoryHealthView, CategoryHealthFix, CategoryDefrag}
	if len(r.Categories) != len(want) {
		t.Fatalf("category count = %d, want %d: %+v", len(r.Categories), len(want), r.Categories)
	}
	for i, category := range want {
		if r.Categories[i].Category != category {
			t.Fatalf("category[%d] = %s, want %s", i, r.Categories[i].Category, category)
		}
	}
}

func TestBuildReportViewServerStateOnly(t *testing.T) {
	// A login with VIEW SERVER STATE but nothing else: metrics works (and so does
	// health view, since VSS is the server-wide alternative), but inspection still
	// needs VIEW ANY DEFINITION and fixes/defrag are missing.
	r := BuildReport(probe{login: "dbadmin", viewServerState: true, engineEdition: 3, currentDatabase: "OIPA"}, "msdb")

	if m := findCategory(t, r, CategoryMetrics); !m.Granted {
		t.Errorf("metrics should be granted with VIEW SERVER STATE")
	}
	if hv := findCategory(t, r, CategoryHealthView); !hv.Granted {
		t.Errorf("health view should be granted with VIEW SERVER STATE")
	}
	insp := findCategory(t, r, CategoryInspection)
	if insp.Granted {
		t.Errorf("inspection should be ungranted without VIEW ANY DEFINITION")
	}
	if got := insp.MissingPermissions; len(got) != 1 || got[0] != "VIEW ANY DEFINITION" {
		t.Errorf("inspection missing = %v, want [VIEW ANY DEFINITION]", got)
	}
	if fix := findCategory(t, r, CategoryHealthFix); fix.Granted {
		t.Errorf("health fix should be ungranted without ALTER")
	}
}

func TestBuildReportRestrictedLoginListsGrants(t *testing.T) {
	r := BuildReport(probe{login: "appuser", engineEdition: 2, currentDatabase: "OIPA"}, "msdb")
	if r.AllGranted() {
		t.Fatalf("restricted login should have no granted categories")
	}

	metrics := findCategory(t, r, CategoryMetrics)
	if want := "GRANT VIEW SERVER STATE TO [appuser];"; metrics.GrantStatements[0] != want {
		t.Errorf("metrics grant = %q, want %q", metrics.GrantStatements[0], want)
	}

	healthView := findCategory(t, r, CategoryHealthView)
	if !strings.Contains(healthView.GrantStatements[0], "USE [OIPA]; GRANT VIEW DATABASE STATE TO [appuser];") {
		t.Errorf("health view grant = %q, want USE [OIPA] + GRANT VIEW DATABASE STATE", healthView.GrantStatements[0])
	}
	if !strings.Contains(healthView.GrantStatements[1], "server-wide alternative") {
		t.Errorf("health view should offer server-wide alternative, got %q", healthView.GrantStatements[1])
	}

	healthFix := findCategory(t, r, CategoryHealthFix)
	if !strings.Contains(healthFix.GrantStatements[0], "USE [OIPA]; GRANT ALTER TO [appuser];") {
		t.Errorf("health fix grant = %q, want USE [OIPA] + GRANT ALTER", healthFix.GrantStatements[0])
	}
	if !strings.Contains(healthFix.GrantStatements[1], "db_ddladmin") {
		t.Errorf("health fix should offer db_ddladmin broad alternative, got %q", healthFix.GrantStatements[1])
	}

	defrag := findCategory(t, r, CategoryDefrag)
	if !strings.Contains(defrag.GrantStatements[0], "USE [msdb]; GRANT CREATE PROCEDURE TO [appuser];") {
		t.Errorf("defrag grant = %q, want USE [msdb] + GRANT CREATE PROCEDURE", defrag.GrantStatements[0])
	}
	if last := defrag.GrantStatements[len(defrag.GrantStatements)-1]; !strings.Contains(last, "db_owner") {
		t.Errorf("defrag should offer db_owner broad alternative, got %q", last)
	}
}

func TestBuildReportHealthFixEngineEditionNote(t *testing.T) {
	standard := findCategory(t, BuildReport(probe{login: "u", alterCurrentDB: true, engineEdition: 2}, "msdb"), CategoryHealthFix)
	if !strings.Contains(standard.Note, "OFFLINE") {
		t.Errorf("Standard edition (2) should note OFFLINE rebuild, got %q", standard.Note)
	}
	enterprise := findCategory(t, BuildReport(probe{login: "u", alterCurrentDB: true, engineEdition: 3}, "msdb"), CategoryHealthFix)
	if strings.Contains(enterprise.Note, "OFFLINE") {
		t.Errorf("Enterprise edition (3) should not note OFFLINE, got %q", enterprise.Note)
	}
}

func TestBuildReportDefragIndividualPermissions(t *testing.T) {
	defrag := findCategory(t, BuildReport(probe{login: "u", createProcedure: true, createTable: true, alterMaintDB: true, engineEdition: 3}, "msdb"), CategoryDefrag)
	if !defrag.Granted {
		t.Errorf("CREATE PROCEDURE + CREATE TABLE + ALTER on maintenance DB should grant defrag")
	}
}

func TestBuildReportDefragPartialPermissions(t *testing.T) {
	defrag := findCategory(t, BuildReport(probe{login: "u", createProcedure: true, createTable: true, engineEdition: 3}, "msdb"), CategoryDefrag)
	if defrag.Granted {
		t.Fatalf("defrag should be ungranted without ALTER on maintenance DB")
	}
	if got := defrag.MissingPermissions; len(got) != 1 || got[0] != "ALTER" {
		t.Fatalf("defrag missing = %v, want [ALTER]", got)
	}
	if got := defrag.GrantStatements[0]; got != "USE [msdb]; GRANT ALTER TO [u];" {
		t.Fatalf("defrag grant = %q, want ALTER only", got)
	}
}

func TestBuildReportDefragOwnerShortcut(t *testing.T) {
	// db_owner/CONTROL on the maintenance DB grants defrag even without the individual perms.
	defrag := findCategory(t, BuildReport(probe{login: "u", isMaintDBOwner: true, engineEdition: 3}, "msdb"), CategoryDefrag)
	if !defrag.Granted {
		t.Errorf("db_owner on maintenance DB should grant defrag")
	}
}

func TestBracketNameEscapesClosingBracket(t *testing.T) {
	if got := bracketName("we]rd"); got != "[we]]rd]" {
		t.Errorf("bracketName escape = %q, want [we]]rd]", got)
	}
}

func findCategory(t *testing.T, r Report, category Category) CategoryResult {
	t.Helper()
	for _, c := range r.Categories {
		if c.Category == category {
			return c
		}
	}
	t.Fatalf("category %s not found", category)
	return CategoryResult{}
}
