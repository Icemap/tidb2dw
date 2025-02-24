package databrickssql

import (
	"fmt"
	"github.com/pingcap-inc/tidb2dw/pkg/tidbsql"
	"github.com/pingcap/errors"
	timodel "github.com/pingcap/tidb/parser/model"
	"github.com/pingcap/tiflow/pkg/sink/cloudstorage"
	"strings"
)

func GenDDLViaColumnsDiff(prevColumns []cloudstorage.TableCol, curTableDef cloudstorage.TableDefinition) ([]string, error) {
	if curTableDef.Type == timodel.ActionTruncateTable {
		return []string{fmt.Sprintf("TRUNCATE TABLE %s", curTableDef.Table)}, nil
	}
	if curTableDef.Type == timodel.ActionDropTable {
		return []string{fmt.Sprintf("DROP TABLE %s", curTableDef.Table)}, nil
	}
	if curTableDef.Type == timodel.ActionCreateTable {
		return nil, errors.New("Received create table ddl, which should not happen") // FIXME: drop table and create table
	}
	if curTableDef.Type == timodel.ActionRenameTables {
		return nil, errors.New("Received rename table ddl, new change data can not be capture by TiCDC any more." +
			"If you want to rename table, please start a new task to capture the new table") // FIXME: rename table to new table and rename back
	}
	if curTableDef.Type == timodel.ActionDropSchema {
		return []string{fmt.Sprintf("DROP SCHEMA %s CASCADE", curTableDef.Schema)}, nil
	}
	if curTableDef.Type == timodel.ActionCreateSchema {
		return nil, errors.New("Received create schema ddl, which should not happen") // FIXME: drop schema and create schema
	}

	columnDiff, err := tidbsql.GetColumnDiff(prevColumns, curTableDef.Columns)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ddls := make([]string, 0, len(columnDiff))
	for _, item := range columnDiff {
		ddl := ""
		switch item.Action {
		case tidbsql.ADD_COLUMN:
			ddl += fmt.Sprintf("ALTER TABLE %s ADD COLUMN ", curTableDef.Table)
			colStr, err := GetDatabricksColumnString(*item.After)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ddl += colStr
		case tidbsql.DROP_COLUMN:
			ddl += fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", curTableDef.Table, item.Before.Name)
		// Databricks does not support direct data type modify
		case tidbsql.MODIFY_COLUMN:
			return nil, errors.New("Received modify column ddl, which is not supported by Databricks yet")
		case tidbsql.RENAME_COLUMN:
			ddl += fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", curTableDef.Table, item.Before.Name, item.After.Name)
		default:
			// UNCHANGE
		}
		if ddl != "" {
			ddl += ";"
			ddls = append(ddls, ddl)
		}
	}

	// TODO: handle primary key
	return ddls, nil
}

// GetDatabricksColumnString returns a string describing the column in Databricks, e.g.
// "id INT NOT NULL DEFAULT '0'"
// Refer to:
// https://dev.mysql.com/doc/refman/8.0/en/data-types.html
// https://docs.databricks.com/en/sql/language-manual/sql-ref-datatypes.html
func GetDatabricksColumnString(column cloudstorage.TableCol) (string, error) {
	var sb strings.Builder
	typeStr, err := GetDatabricksTypeString(column)
	if err != nil {
		return "", errors.Trace(err)
	}
	sb.WriteString(fmt.Sprintf("%s %s", column.Name, typeStr))
	if column.Nullable == "false" {
		sb.WriteString(" NOT NULL")
	}
	// Delta table not support default value yet.
	return sb.String(), nil
}
