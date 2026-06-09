package sqlperms

import "testing"

func TestBuildReportTraceGrants(t *testing.T) {
	r := BuildReport(probe{login: "app_login", currentDatabase: "OIPA", engineEdition: 2}, "msdb")
	trace := findCategory(t, r, CategoryTrace)
	if trace.Granted {
		t.Fatalf("trace category granted = true, want false")
	}
	wantMissing := []string{"ALTER ANY EVENT SESSION", "VIEW SERVER STATE"}
	for _, want := range wantMissing {
		if !contains(trace.MissingPermissions, want) {
			t.Fatalf("trace missing permissions = %v, want %s", trace.MissingPermissions, want)
		}
	}
	wantGrant := "GRANT ALTER ANY EVENT SESSION TO [app_login];"
	if !contains(trace.GrantStatements, wantGrant) {
		t.Fatalf("trace grants = %v, want %q", trace.GrantStatements, wantGrant)
	}
}

func TestBuildReportSysadminGranted(t *testing.T) {
	r := BuildReport(probe{login: "sa", isSysadmin: true, engineEdition: 3}, "msdb")
	for _, c := range r.Categories {
		if !c.Granted {
			t.Fatalf("category %s granted = false, want true", c.Category)
		}
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

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
