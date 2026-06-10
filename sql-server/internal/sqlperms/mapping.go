package sqlperms

import "fmt"

// enterpriseEngineEditions are SERVERPROPERTY('EngineEdition') values that
// support ONLINE/RESUMABLE index rebuilds (Enterprise/Developer, Azure SQL DB,
// Azure SQL Managed Instance).
var enterpriseEngineEditions = map[int]bool{3: true, 5: true, 8: true}

// BuildReport maps collected probe facts to per-category verdicts.
func BuildReport(p probe, maintenanceDatabase string) Report {
	r := Report{
		Login:               p.login,
		MaintenanceDatabase: maintenanceDatabase,
		IsSysadmin:          p.isSysadmin,
		EngineEdition:       p.engineEdition,
		Warnings:            p.warnings,
	}
	r.Categories = []CategoryResult{
		metricsResult(p),
		inspectionResult(p),
		healthViewResult(p),
		healthFixResult(p),
		defragResult(p, maintenanceDatabase),
	}
	return r
}

func metricsResult(p probe) CategoryResult {
	c := CategoryResult{Category: CategoryMetrics, Label: "Server metrics (CPU, memory, IOPS, disk)"}
	if p.isSysadmin || p.viewServerState {
		return granted(c, p)
	}
	c.MissingPermissions = []string{"VIEW SERVER STATE"}
	c.GrantStatements = []string{grantServer("VIEW SERVER STATE", p.login)}
	return c
}

func inspectionResult(p probe) CategoryResult {
	c := CategoryResult{Category: CategoryInspection, Label: "Database, schema, and session inspection"}
	if p.isSysadmin {
		return granted(c, p)
	}
	if !p.viewServerState {
		c.MissingPermissions = append(c.MissingPermissions, "VIEW SERVER STATE")
		c.GrantStatements = append(c.GrantStatements, grantServer("VIEW SERVER STATE", p.login))
	}
	if !p.viewAnyDefinition {
		c.MissingPermissions = append(c.MissingPermissions, "VIEW ANY DEFINITION")
		c.GrantStatements = append(c.GrantStatements, grantServer("VIEW ANY DEFINITION", p.login))
	}
	if len(c.MissingPermissions) == 0 {
		return granted(c, p)
	}
	return c
}

func healthViewResult(p probe) CategoryResult {
	c := CategoryResult{Category: CategoryHealthView, Label: "Index health & fragmentation scan"}
	if p.isSysadmin || p.viewServerState || p.viewDatabaseState {
		return granted(c, p)
	}
	c.MissingPermissions = []string{"VIEW DATABASE STATE"}
	c.GrantStatements = []string{
		useDB(p.currentDatabase) + grantCurrent("VIEW DATABASE STATE", p.login),
		grantServer("VIEW SERVER STATE", p.login) + " -- server-wide alternative",
	}
	return c
}

func healthFixResult(p probe) CategoryResult {
	c := CategoryResult{Category: CategoryHealthFix, Label: "Apply index fixes (rebuild, reorganize, drop, update stats)"}
	if !enterpriseEngineEditions[p.engineEdition] {
		c.Note = "ONLINE/RESUMABLE rebuild unavailable on this edition; rebuilds run OFFLINE."
	}
	if p.isSysadmin || p.alterCurrentDB {
		c.Granted = true
		if p.isSysadmin {
			c.Note = joinNote("granted via sysadmin", c.Note)
		}
		return c
	}
	c.MissingPermissions = []string{"ALTER"}
	c.GrantStatements = []string{
		useDB(p.currentDatabase) + grantCurrent("ALTER", p.login),
		useDB(p.currentDatabase) + addRole("db_ddladmin", p.login) + " -- broader role alternative",
	}
	return c
}

func defragResult(p probe, maintenanceDatabase string) CategoryResult {
	c := CategoryResult{Category: CategoryDefrag, Label: "Install & run AdaptiveIndexDefrag"}
	if p.isSysadmin || p.isMaintDBOwner || (p.createProcedure && p.createTable && p.alterMaintDB) {
		return granted(c, p)
	}
	use := useDB(maintenanceDatabase)
	if !p.createProcedure {
		c.MissingPermissions = append(c.MissingPermissions, "CREATE PROCEDURE")
		c.GrantStatements = append(c.GrantStatements, use+grantCurrent("CREATE PROCEDURE", p.login))
	}
	if !p.createTable {
		c.MissingPermissions = append(c.MissingPermissions, "CREATE TABLE")
		c.GrantStatements = append(c.GrantStatements, use+grantCurrent("CREATE TABLE", p.login))
	}
	if !p.alterMaintDB {
		c.MissingPermissions = append(c.MissingPermissions, "ALTER")
		c.GrantStatements = append(c.GrantStatements, use+grantCurrent("ALTER", p.login))
	}
	c.GrantStatements = append(c.GrantStatements, use+addRole("db_owner", p.login)+" -- broader role alternative")
	return c
}

func granted(c CategoryResult, p probe) CategoryResult {
	c.Granted = true
	if p.isSysadmin {
		c.Note = joinNote("granted via sysadmin", c.Note)
	}
	return c
}

func grantServer(perm, login string) string {
	return fmt.Sprintf("GRANT %s TO %s;", perm, bracketName(login))
}

func grantCurrent(perm, login string) string {
	return fmt.Sprintf("GRANT %s TO %s;", perm, bracketName(login))
}

func addRole(role, login string) string {
	return fmt.Sprintf("ALTER ROLE %s ADD MEMBER %s;", role, bracketName(login))
}

func useDB(database string) string {
	if database == "" {
		return ""
	}
	return "USE " + bracketName(database) + "; "
}

func joinNote(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += p
	}
	return out
}
