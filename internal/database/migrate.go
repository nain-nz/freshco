package database

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	migmysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"smartfood/internal/config"
)

func newMigrate(cfg config.Config) (*migrate.Migrate, error) {
	db, err := Open(cfg)
	if err != nil {
		return nil, err
	}
	src, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	drv, err := migmysql.WithInstance(db, &migmysql.Config{})
	if err != nil {
		return nil, err
	}
	return migrate.NewWithInstance("iofs", src, "mysql", drv)
}

// Migrate menjalankan perintah migration (up, down, fresh, version, force).
func Migrate(cfg config.Config, args []string) error {
	if err := EnsureDatabase(cfg); err != nil {
		return fmt.Errorf("gagal membuat database: %w", err)
	}

	cmd := "up"
	if len(args) > 0 {
		cmd = args[0]
	}

	if cmd == "fresh" {
		m, err := newMigrate(cfg)
		if err != nil {
			return err
		}
		if err := m.Drop(); err != nil {
			return err
		}
		m.Close()
		fmt.Println("semua tabel dihapus")
		cmd = "up"
	}

	m, err := newMigrate(cfg)
	if err != nil {
		return err
	}
	defer m.Close()

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		n := 1
		if len(args) > 1 {
			if n, err = strconv.Atoi(args[1]); err != nil || n < 1 {
				return errors.New("jumlah langkah harus angka >= 1")
			}
		}
		err = m.Steps(-n)
	case "version":
		v, dirty, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			fmt.Println("belum ada migration yang dijalankan")
			return nil
		}
		if verr != nil {
			return verr
		}
		fmt.Printf("versi: %d, dirty: %v\n", v, dirty)
		return nil
	case "force":
		if len(args) < 2 {
			return errors.New("pemakaian: migrate force <versi>")
		}
		v, aerr := strconv.Atoi(args[1])
		if aerr != nil {
			return errors.New("versi harus angka")
		}
		err = m.Force(v)
	default:
		return fmt.Errorf("perintah migrate tidak dikenal: %s", cmd)
	}

	switch {
	case err == nil:
		fmt.Println("migration selesai")
		return nil
	case errors.Is(err, migrate.ErrNoChange):
		fmt.Println("tidak ada migration baru")
		return nil
	case errors.Is(err, os.ErrNotExist):
		fmt.Println("tidak ada migration yang bisa di-rollback")
		return nil
	}
	return err
}
