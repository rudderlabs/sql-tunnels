package ssh

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
)

func init() {
	sql.Register("ssh", &Driver{})
}

var _ driver.Driver = (*Driver)(nil)

type Driver struct{}

func (d *Driver) Open(name string) (driver.Conn, error) {
	return nil, fmt.Errorf("not implemented")
}
