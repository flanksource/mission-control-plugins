package sqlinspect

import (
	"fmt"
	"time"

	archsql "github.com/flanksource/arch-unit/analysis/sql"
	"github.com/flanksource/arch-unit/models/uir"
	"gorm.io/gorm"
)

func Extract(db *gorm.DB) (uir.UIR, error) {
	if db == nil {
		return uir.UIR{}, fmt.Errorf("nil db")
	}
	extractor := archsql.NewSQLASTExtractor()
	extractor.ConnectTimeout = 10 * time.Second
	return extractor.Extract(archsql.ExtractOptions{Gorm: db})
}
