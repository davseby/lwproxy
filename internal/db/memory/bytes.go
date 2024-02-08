package memory

import "context"

// FetchBytes fetches bytes from the database.
func (d *DB) FetchBytes(_ context.Context) (int64, error) {
	return d.bytes.Load(), nil
}

// IncreaseBytes increases the amount of bytes used.
func (d *DB) IncreaseBytes(_ context.Context, usedBytes int64) error {
	d.bytes.Add(usedBytes)
	return nil
}
