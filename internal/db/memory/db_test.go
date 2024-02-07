package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewDB(t *testing.T) {
	db := NewDB()
	assert.NotNil(t, db)
	assert.NotNil(t, db.bytes)
}
